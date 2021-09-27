//go:build windows
// +build windows

package platform

// CheckOutOfFileDescriptors on Windows is not used.
func CheckOutOfFileDescriptors() error {
	return nil
}
