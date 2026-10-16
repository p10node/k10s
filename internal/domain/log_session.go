package domain

// LogSession binds history and the live stream to one resolved pod/container.
// Lines includes the initial tail; consumers must not fetch a separate tail.
type LogSession interface {
	Lines() <-chan string
	History(n int) ([]string, bool, error)
	Close()
}

// LogOpener is optional so offline/third-party Sources remain compatible.
type LogOpener interface {
	OpenLogSession(kind, namespace, name string, tail int) (LogSession, error)
}
