package share

import "testing"

func TestApplicationWebPortsStayPrivateToShareLinks(t *testing.T) {
	protected := map[int]bool{8400: true}
	service := New(nil, nil, WithProtectedPort(func(port int) bool { return protected[port] }))
	if err := service.ShareablePort(8400); err != ErrPortNotShareable {
		t.Fatalf("web port: %v", err)
	}
	if err := service.ShareablePort(3000); err != nil {
		t.Fatalf("dev port: %v", err)
	}
	protected[3000] = true
	if err := service.ShareablePort(3000); err != ErrPortNotShareable {
		t.Fatalf("new web port: %v", err)
	}
}
