//go:build !linux

package credentials

import (
	"fmt"
	"time"
)

type unsupportedKeys struct{}

func newKeyStore() keyStore { return unsupportedKeys{} }
func (unsupportedKeys) Read(string) ([]byte, bool, error) {
	return nil, false, fmt.Errorf("session keyring requires Linux")
}
func (unsupportedKeys) Write(string, []byte, time.Duration) error {
	return fmt.Errorf("session keyring requires Linux")
}
func (unsupportedKeys) Delete(string) error { return fmt.Errorf("session keyring requires Linux") }
func readSourceKey(string) (string, bool, error) {
	return "", false, fmt.Errorf("keyring provider requires Linux")
}
