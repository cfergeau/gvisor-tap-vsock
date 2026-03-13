//go:build !(darwin && arm64)

package e2eutils

func efiArgs() ([]string, error) {
	return nil, nil
}
