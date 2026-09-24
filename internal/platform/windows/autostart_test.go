package windows

import "testing"

func TestLaunchCommandQuotesPathWithSpaces(t *testing.T) {
	got := launchCommand(`C:\Program Files\PopTrans\PopTrans.exe`)
	want := `"C:\Program Files\PopTrans\PopTrans.exe" --autostart`
	if got != want {
		t.Fatalf("launchCommand = %s, want %s", got, want)
	}
}

func TestLaunchCommandKeepsSimplePath(t *testing.T) {
	got := launchCommand(`D:\tools\PopTrans.exe`)
	want := `"D:\tools\PopTrans.exe" --autostart`
	if got != want {
		t.Fatalf("launchCommand = %s, want %s", got, want)
	}
}
