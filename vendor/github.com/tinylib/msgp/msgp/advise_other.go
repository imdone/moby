// +build !linux appengine

package msgp

import (
	"os"
)

// TODO: darwin, BSD support id:962 gh:963

func adviseRead(mem []byte) {}

func adviseWrite(mem []byte) {}

func fallocate(f *os.File, sz int64) error {
	return f.Truncate(sz)
}
