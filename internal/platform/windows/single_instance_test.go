package windows

import (
	"fmt"
	"os"
	"testing"
)

func TestAcquireSingleInstance(t *testing.T) {
	name := fmt.Sprintf(`Local\PopTrans.Test.%d`, os.Getpid())
	already, err := AcquireSingleInstance(name)
	if err != nil {
		t.Fatalf("first acquire: %v", err)
	}
	if already {
		t.Fatal("first acquire reported an existing instance")
	}

	second, err := AcquireSingleInstance(name)
	if err != nil {
		t.Fatalf("second acquire: %v", err)
	}
	if !second {
		t.Fatal("second acquire did not detect the first instance")
	}

	ReleaseSingleInstance()
	third, err := AcquireSingleInstance(name)
	if err != nil {
		t.Fatalf("third acquire: %v", err)
	}
	if third {
		t.Fatal("instance still reported after release")
	}
	ReleaseSingleInstance()
}
