package update

import (
	"archive/tar"
	"archive/zip"
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"
)

// maxDownload caps a release download. A k10s archive is tens of megabytes;
// half a gigabyte means something has gone wrong and should stop, not fill
// the disk.
const maxDownload = 512 << 20

// Progress is called while the asset downloads. total is -1 when the server
// sends no Content-Length.
type Progress func(done, total int64)

// Result describes a completed install.
type Result struct {
	Version string
	Path    string // the executable that was replaced
	Backup  string // set on Windows, where the previous binary can't be deleted yet
}

// Apply downloads rel's asset for this platform, verifies it against the
// release checksums, and replaces the running executable with it.
//
// The swap is a rename within the executable's own directory, so it is
// atomic and never leaves a half-written binary behind: on POSIX the running
// process keeps its open inode and keeps working, and the next launch is the
// new build.
func Apply(ctx context.Context, c *Client, rel *Release, progress Progress) (Result, error) {
	var res Result
	if rel == nil {
		return res, fmt.Errorf("no release to install")
	}
	goos, goarch := c.platform()
	if !rel.HasAsset() {
		return res, fmt.Errorf("release %s has no build for %s/%s (%s)", rel.Tag, goos, goarch, rel.AssetHint())
	}

	exe, err := c.target()
	if err != nil {
		return res, err
	}
	dir := filepath.Dir(exe)
	if err := checkWritable(dir); err != nil {
		return res, err
	}
	res.Version, res.Path = rel.Version, exe

	// Downloaded into the destination directory so the final rename stays on
	// one filesystem — across devices it is a copy, which is not atomic.
	dl, err := os.CreateTemp(dir, ".k10s-download-*")
	if err != nil {
		return res, err
	}
	dlPath := dl.Name()
	defer os.Remove(dlPath)

	sum, err := download(ctx, c, rel.Asset, dl, progress)
	dl.Close()
	if err != nil {
		return res, err
	}

	if err := verify(ctx, c, rel, sum); err != nil {
		return res, err
	}

	bin, err := os.CreateTemp(dir, ".k10s-new-*")
	if err != nil {
		return res, err
	}
	binPath := bin.Name()
	defer os.Remove(binPath)

	if err := extract(dlPath, rel.Asset.Name, goos, bin); err != nil {
		bin.Close()
		return res, err
	}
	if err := bin.Close(); err != nil {
		return res, err
	}
	if err := sanityCheck(binPath); err != nil {
		return res, err
	}
	// 0755 and not 0700: a binary in /usr/local/bin is normally runnable by
	// everyone, and an update should not quietly change that.
	if err := os.Chmod(binPath, 0o755); err != nil {
		return res, err
	}

	backup, err := replaceExecutable(binPath, exe)
	if err != nil {
		return res, err
	}
	res.Backup = backup
	return res, nil
}

// target is the file this client installs over.
func (c *Client) target() (string, error) {
	if c != nil && c.Target != "" {
		return c.Target, nil
	}
	return TargetPath()
}

// TargetPath is the file Apply replaces: the running executable with symlinks
// resolved, so updating a `ln -s` in ~/bin rewrites the real binary rather
// than clobbering the link.
func TargetPath() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("cannot locate the running binary: %w", err)
	}
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}
	return exe, nil
}

// checkWritable turns the failure people actually hit — a binary installed
// into a root-owned directory — into a sentence that says what to do.
func checkWritable(dir string) error {
	f, err := os.CreateTemp(dir, ".k10s-write-test-*")
	if err != nil {
		return fmt.Errorf("cannot write to %s — reinstall through your package manager, or re-run with sudo: %w", dir, err)
	}
	name := f.Name()
	f.Close()
	os.Remove(name)
	return nil
}

// download streams asset into w and returns its SHA-256.
func download(ctx context.Context, c *Client, asset Asset, w io.Writer, progress Progress) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, asset.URL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/octet-stream")
	req.Header.Set("User-Agent", "k10s-updater")

	resp, err := c.httpClient().Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("downloading %s: %s", asset.Name, resp.Status)
	}

	total := resp.ContentLength
	if total <= 0 && asset.Size > 0 {
		total = asset.Size
	}
	h := sha256.New()
	dst := io.MultiWriter(w, h)
	if progress != nil {
		dst = io.MultiWriter(w, h, &progressWriter{fn: progress, total: total})
	}
	n, err := io.Copy(dst, io.LimitReader(resp.Body, maxDownload+1))
	if err != nil {
		return "", err
	}
	if n > maxDownload {
		return "", fmt.Errorf("%s is larger than the %d MiB download limit", asset.Name, maxDownload>>20)
	}
	if n == 0 {
		return "", fmt.Errorf("%s downloaded empty", asset.Name)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

type progressWriter struct {
	fn    Progress
	done  int64
	total int64
}

func (p *progressWriter) Write(b []byte) (int, error) {
	p.done += int64(len(b))
	p.fn(p.done, p.total)
	return len(b), nil
}

// verify checks the download against the release's checksum manifest. A
// release without one installs unverified — noted in docs/update.md — but a
// manifest that exists and disagrees, or that doesn't mention this asset at
// all, stops the install.
func verify(ctx context.Context, c *Client, rel *Release, sum string) error {
	if rel.Sums.URL == "" {
		return nil
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rel.Sums.URL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "k10s-updater")
	resp, err := c.httpClient().Do(req)
	if err != nil {
		return fmt.Errorf("fetching %s: %w", rel.Sums.Name, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("fetching %s: %s", rel.Sums.Name, resp.Status)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return err
	}
	want, ok := findSum(string(body), rel.Asset.Name)
	if !ok {
		return fmt.Errorf("%s does not list %s — refusing to install unverified", rel.Sums.Name, rel.Asset.Name)
	}
	if !strings.EqualFold(want, sum) {
		return fmt.Errorf("checksum mismatch for %s: expected %s, got %s", rel.Asset.Name, want, sum)
	}
	return nil
}

// findSum reads the `sha256sum` output format: one "<hex>  <name>" per line,
// where the name may carry a "*" binary marker or a directory prefix.
func findSum(manifest, asset string) (string, bool) {
	sc := bufio.NewScanner(strings.NewReader(manifest))
	for sc.Scan() {
		fields := strings.Fields(sc.Text())
		if len(fields) < 2 {
			continue
		}
		name := strings.TrimPrefix(fields[len(fields)-1], "*")
		if path.Base(name) == asset {
			return fields[0], true
		}
	}
	return "", false
}

// BinaryName is the file to look for inside a release archive.
func BinaryName(goos string) string {
	if goos == "windows" {
		return "k10s.exe"
	}
	return "k10s"
}

// extract copies the k10s binary out of the downloaded file into w. A bare
// binary (no archive extension) is copied straight through.
func extract(archivePath, assetName, goos string, w io.Writer) error {
	switch archiveKind(assetName) {
	case ".tar.gz":
		return extractTarGz(archivePath, BinaryName(goos), w)
	case ".zip":
		return extractZip(archivePath, BinaryName(goos), w)
	default:
		f, err := os.Open(archivePath)
		if err != nil {
			return err
		}
		defer f.Close()
		_, err = io.Copy(w, f)
		return err
	}
}

// extractTarGz copies the k10s entry out of the archive. If no entry carries
// that name — an archive that renamed the binary — the largest regular file
// is taken instead, which is invariably it.
func extractTarGz(archivePath, want string, w io.Writer) error {
	name, err := findTarEntry(archivePath, want)
	if err != nil {
		return err
	}
	return walkTarGz(archivePath, func(h *tar.Header, r io.Reader) (bool, error) {
		if h.Name != name {
			return false, nil
		}
		_, err := io.Copy(w, io.LimitReader(r, maxDownload))
		return true, err
	})
}

// findTarEntry names the entry to extract: the one called want, else the
// largest regular file.
func findTarEntry(archivePath, want string) (string, error) {
	var biggest string
	var size int64
	var exact string
	err := walkTarGz(archivePath, func(h *tar.Header, _ io.Reader) (bool, error) {
		if h.Typeflag != tar.TypeReg {
			return false, nil
		}
		if path.Base(h.Name) == want {
			exact = h.Name
			return true, nil
		}
		if h.Size > size {
			biggest, size = h.Name, h.Size
		}
		return false, nil
	})
	if err != nil {
		return "", err
	}
	if exact != "" {
		return exact, nil
	}
	if biggest == "" {
		return "", fmt.Errorf("archive contains no %s", want)
	}
	return biggest, nil
}

// walkTarGz calls fn for each entry until fn reports it is done.
func walkTarGz(archivePath string, fn func(*tar.Header, io.Reader) (bool, error)) error {
	f, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return fmt.Errorf("not a gzip archive: %w", err)
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	for {
		h, err := tr.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		done, err := fn(h, tr)
		if err != nil || done {
			return err
		}
	}
}

// extractZip is the same rule as extractTarGz — the entry named k10s, else
// the largest one — against a zip, which is what a Windows release ships.
func extractZip(archivePath, want string, w io.Writer) error {
	zr, err := zip.OpenReader(archivePath)
	if err != nil {
		return fmt.Errorf("not a zip archive: %w", err)
	}
	defer zr.Close()

	var target, biggest *zip.File
	for _, f := range zr.File {
		if f.FileInfo().IsDir() {
			continue
		}
		if path.Base(f.Name) == want {
			target = f
			break
		}
		if biggest == nil || f.UncompressedSize64 > biggest.UncompressedSize64 {
			biggest = f
		}
	}
	if target == nil {
		target = biggest
	}
	if target == nil {
		return fmt.Errorf("archive contains no %s", want)
	}
	rc, err := target.Open()
	if err != nil {
		return err
	}
	defer rc.Close()
	_, err = io.Copy(w, io.LimitReader(rc, maxDownload))
	return err
}

// sanityCheck refuses to install something that is not an executable for
// this platform — an HTML error page served in place of an asset is the
// realistic case, and it would otherwise be renamed over a working binary.
func sanityCheck(binPath string) error {
	f, err := os.Open(binPath)
	if err != nil {
		return err
	}
	defer f.Close()
	fi, err := f.Stat()
	if err != nil {
		return err
	}
	if fi.Size() < 1024 {
		return fmt.Errorf("downloaded binary is only %d bytes — refusing to install it", fi.Size())
	}
	head := make([]byte, 4)
	if _, err := io.ReadFull(f, head); err != nil {
		return err
	}
	if !looksExecutable(head) {
		return fmt.Errorf("downloaded file is not an executable (starts with %q)", head)
	}
	return nil
}

// Magic numbers: ELF, the four Mach-O flavours plus a universal binary, and
// the DOS header every PE still starts with.
var execMagic = [][]byte{
	{0x7f, 'E', 'L', 'F'},
	{0xfe, 0xed, 0xfa, 0xce}, {0xfe, 0xed, 0xfa, 0xcf},
	{0xce, 0xfa, 0xed, 0xfe}, {0xcf, 0xfa, 0xed, 0xfe},
	{0xca, 0xfe, 0xba, 0xbe}, {0xbe, 0xba, 0xfe, 0xca},
	{'M', 'Z'},
}

func looksExecutable(head []byte) bool {
	for _, m := range execMagic {
		if bytes.HasPrefix(head, m) {
			return true
		}
	}
	return false
}

// replaceExecutable swaps newPath in for exe, returning the path of any
// backup left behind.
//
// POSIX renames over a running binary happily — the kernel holds the old
// inode until the process exits. Windows refuses, so the running file is
// moved aside first and deleted on a later run.
func replaceExecutable(newPath, exe string) (string, error) {
	if runtime.GOOS != "windows" {
		return "", os.Rename(newPath, exe)
	}
	backup := exe + ".old"
	os.Remove(backup)
	if err := os.Rename(exe, backup); err != nil {
		return "", fmt.Errorf("moving the running binary aside: %w", err)
	}
	if err := os.Rename(newPath, exe); err != nil {
		os.Rename(backup, exe) // put the working binary back
		return "", err
	}
	// Expected to fail while the old image is still mapped; the next update
	// removes it.
	if os.Remove(backup) == nil {
		return "", nil
	}
	return backup, nil
}
