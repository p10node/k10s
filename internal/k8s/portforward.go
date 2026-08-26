package k8s

import (
	"fmt"
	"io"
	"net"
	"net/http"

	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/tools/portforward"
	"k8s.io/client-go/transport/spdy"
)

// PortForward starts forwarding an ephemeral local port to the pod's first
// container port (like `kubectl port-forward`) and returns the local
// address plus a func to stop it. Services are resolved to one backing pod.
func (s *Store) PortForward(kind, ns, name string) (string, func(), error) {
	ens := effectiveNS(ns)
	podName, remotePort, err := s.portForwardTarget(kind, ens, name)
	if err != nil {
		return "", nil, err
	}

	localPort, err := freeLocalPort()
	if err != nil {
		return "", nil, err
	}

	req := s.c.Clientset.CoreV1().RESTClient().Post().
		Resource("pods").Namespace(ens).Name(podName).SubResource("portforward")
	transport, upgrader, err := spdy.RoundTripperFor(s.c.RestConfig)
	if err != nil {
		return "", nil, err
	}
	dialer := spdy.NewDialer(upgrader, &http.Client{Transport: transport}, "POST", req.URL())

	stopCh := make(chan struct{})
	readyCh := make(chan struct{})
	errCh := make(chan error, 1)

	fw, err := portforward.New(dialer, []string{fmt.Sprintf("%d:%d", localPort, remotePort)}, stopCh, readyCh, io.Discard, io.Discard)
	if err != nil {
		return "", nil, err
	}

	go func() {
		if err := fw.ForwardPorts(); err != nil {
			errCh <- err
		}
	}()

	select {
	case <-readyCh:
	case err := <-errCh:
		return "", nil, err
	}

	addr := fmt.Sprintf("localhost:%d", localPort)
	stop := func() { close(stopCh) }
	return addr, stop, nil
}

// portForwardTarget resolves the pod name and remote port to dial: for a
// pod, its first container port; for a service, one ready backing pod and
// the service's first port.
func (s *Store) portForwardTarget(kind, ns, name string) (pod string, port int, err error) {
	switch kind {
	case "pods":
		p, err := s.podLister().Pods(ns).Get(name)
		if err != nil {
			return "", 0, err
		}
		if len(p.Spec.Containers) == 0 || len(p.Spec.Containers[0].Ports) == 0 {
			return "", 0, fmt.Errorf("%s exposes no container ports", name)
		}
		return p.Name, int(p.Spec.Containers[0].Ports[0].ContainerPort), nil
	case "services":
		svc, err := s.svcLister().Services(ns).Get(name)
		if err != nil {
			return "", 0, err
		}
		if len(svc.Spec.Ports) == 0 {
			return "", 0, fmt.Errorf("%s exposes no ports", name)
		}
		sel := labels.SelectorFromSet(labels.Set(svc.Spec.Selector))
		pods, err := s.podLister().Pods(ns).List(sel)
		if err != nil || len(pods) == 0 {
			return "", 0, fmt.Errorf("no backing pods found for service %s", name)
		}
		port := svc.Spec.Ports[0].TargetPort.IntValue()
		if port == 0 {
			port = int(svc.Spec.Ports[0].Port)
		}
		return pods[0].Name, port, nil
	}
	return "", 0, fmt.Errorf("port-forward is not supported for %s", kind)
}

func freeLocalPort() (int, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port, nil
}
