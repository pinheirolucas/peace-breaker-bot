package server

import (
	"fmt"
	"log/slog"
	"os"

	"github.com/libp2p/zeroconf/v2"
)

const (
	autodiscoveryPathRecord = "path=/api"
	autodiscoveryAPIRecord  = "api=1"
)

func newAutodiscoveryRegistration(port int) (string, []string, error) {
	hostname, err := os.Hostname()
	if err != nil {
		return "", nil, err
	}

	instanceName := fmt.Sprintf(
		"%s-%d",
		hostname,
		port,
	)

	text := []string{
		autodiscoveryPathRecord,
		autodiscoveryAPIRecord,
	}

	return instanceName, text, nil
}

func newAutodiscoveryServer(service string, port int) (*zeroconf.Server, error) {
	instanceName, text, err := newAutodiscoveryRegistration(port)
	if err != nil {
		return nil, err
	}

	slog.Debug("autodiscovery registration", "instance", instanceName, "port", port, "txt", text)

	server, err := zeroconf.Register(
		instanceName,
		service,
		"local.",
		port,
		text,
		nil,
	)
	if err != nil {
		return nil, err
	}

	return server, nil
}
