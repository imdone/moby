// +build linux

package quota

import (
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"
)

// 10MB
const testQuotaSize = 10 * 1024 * 1024
const imageSize = 64 * 1024 * 1024

func TestBlockDev(t *testing.T) {
	mkfs, err := exec.LookPath("mkfs.xfs")
	if err != nil {
		t.Fatal("mkfs.xfs not installed")
	}

	// create a sparse image
	imageFile, err := ioutil.TempFile("", "xfs-image")
	if err != nil {
		t.Fatal(err)
	}
	imageFileName := imageFile.Name()
	defer os.Remove(imageFileName)
	if _, err = imageFile.Seek(imageSize-1, 0); err != nil {
		t.Fatal(err)
	}
	if _, err = imageFile.Write([]byte{0}); err != nil {
		t.Fatal(err)
	}
	if err = imageFile.Close(); err != nil {
		t.Fatal(err)
	}

	// The reason for disabling these options is sometimes people run with a newer userspace
	// than kernelspace
	out, err := exec.Command(mkfs, "-m", "crc=0,finobt=0", imageFileName).CombinedOutput()
	if len(out) > 0 {
		t.Log(string(out))
	}
	if err != nil {
		t.Fatal(err)
	}

	runTest(t, "testBlockDevQuotaDisabled", wrapMountTest(imageFileName, false, testBlockDevQuotaDisabled))
	runTest(t, "testBlockDevQuotaEnabled", wrapMountTest(imageFileName, true, testBlockDevQuotaEnabled))
	runTest(t, "testSmallerThanQuota", wrapMountTest(imageFileName, true, wrapQuotaTest(testSmallerThanQuota)))
	runTest(t, "testBiggerThanQuota", wrapMountTest(imageFileName, true, wrapQuotaTest(testBiggerThanQuota)))
	runTest(t, "testRetrieveQuota", wrapMountTest(imageFileName, true, wrapQuotaTest(testRetrieveQuota)))
}

func runTest(t *testing.T, testName string, testFunc func(*testing.T)) {
	if success := t.Run(testName, testFunc); !success {
		out, _ := exec.Command("dmesg").CombinedOutput()
		t.Log(string(out))
	}
}

func wrapMountTest(imageFileName string, enableQuota bool, testFunc func(t *testing.T, mountPoint, backingFsDev string)) func(*testing.T) {
	return func(t *testing.T) {
		mountOptions := "loop"

		if enableQuota {
			mountOptions = mountOptions + ",prjquota"
		}

		// create a mountPoint
		mountPoint, err := ioutil.TempDir("", "xfs-mountPoint")
		if err != nil {
			t.Fatal(err)
		}
		defer os.RemoveAll(mountPoint)

		out, err := exec.Command("mount", "-o", mountOptions, imageFileName, mountPoint).CombinedOutput()
		if len(out) > 0 {
			t.Log(string(out))
		}
		if err != nil {
			t.Fatal("mount failed")
		}

		defer func() {
			if err := unix.Unmount(mountPoint, 0); err != nil {
				t.Fatal(err)
			}
		}()

		backingFsDev, err := makeBackingFsDev(mountPoint)
		require.NoError(t, err)

		testFunc(t, mountPoint, backingFsDev)
	}
}

func testBlockDevQuotaDisabled(t *testing.T, mountPoint, backingFsDev string) {
	hasSupport, err := hasQuotaSupport(backingFsDev)
	require.NoError(t, err)
	assert.False(t, hasSupport)
}

func testBlockDevQuotaEnabled(t *testing.T, mountPoint, backingFsDev string) {
	hasSupport, err := hasQuotaSupport(backingFsDev)
	require.NoError(t, err)
	assert.True(t, hasSupport)
}

func wrapQuotaTest(testFunc func(t *testing.T, ctrl *Control, mountPoint, testDir, testSubDir string)) func(t *testing.T, mountPoint, backingFsDev string) {
	return func(t *testing.T, mountPoint, backingFsDev string) {
		testDir, err := ioutil.TempDir(mountPoint, "per-test")
		require.NoError(t, err)
		defer os.RemoveAll(testDir)

		ctrl, err := NewControl(testDir)
		require.NoError(t, err)

		testSubDir, err := ioutil.TempDir(testDir, "quota-test")
		require.NoError(t, err)
		testFunc(t, ctrl, mountPoint, testDir, testSubDir)
	}

}

func testSmallerThanQuota(t *testing.T, ctrl *Control, homeDir, testDir, testSubDir string) {
	require.NoError(t, ctrl.SetQuota(testSubDir, Quota{testQuotaSize}))
	smallerThanQuotaFile := filepath.Join(testSubDir, "smaller-than-quota")
	require.NoError(t, ioutil.WriteFile(smallerThanQuotaFile, make([]byte, testQuotaSize/2), 0644))
	require.NoError(t, os.Remove(smallerThanQuotaFile))
}

func testBiggerThanQuota(t *testing.T, ctrl *Control, homeDir, testDir, testSubDir string) {
	// Make sure the quota is being enforced
	// TODO: When we implement this under EXT4, we need to shed CAP_SYS_RESOURCE, otherwise id:91 gh:92
	// we're able to violate quota without issue
	require.NoError(t, ctrl.SetQuota(testSubDir, Quota{testQuotaSize}))

	biggerThanQuotaFile := filepath.Join(testSubDir, "bigger-than-quota")
	err := ioutil.WriteFile(biggerThanQuotaFile, make([]byte, testQuotaSize+1), 0644)
	require.Error(t, err)
	if err == io.ErrShortWrite {
		require.NoError(t, os.Remove(biggerThanQuotaFile))
	}
}

func testRetrieveQuota(t *testing.T, ctrl *Control, homeDir, testDir, testSubDir string) {
	// Validate that we can retrieve quota
	require.NoError(t, ctrl.SetQuota(testSubDir, Quota{testQuotaSize}))

	var q Quota
	require.NoError(t, ctrl.GetQuota(testSubDir, &q))
	assert.EqualValues(t, testQuotaSize, q.Size)
}
