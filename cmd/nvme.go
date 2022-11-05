// Copyright 2017-2022 Daniel Swarbrick. All rights reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"flag"
	"fmt"
	"os"
	"runtime"
	"unsafe"

	"github.com/dswarbrick/go-nvme/nvme"

	"golang.org/x/sys/unix"
)

const (
	_LINUX_CAPABILITY_VERSION_3 = 0x20080522

	CAP_SYS_RAWIO = 1 << 17
	CAP_SYS_ADMIN = 1 << 21
)

type capHeader struct {
	version uint32
	pid     int
}

type capData struct {
	effective   uint32
	permitted   uint32 //lint:ignore U1000 unused but required member
	inheritable uint32 //lint:ignore U1000 unused but required member
}

type capsV3 struct {
	hdr  capHeader
	data [2]capData
}

// checkCaps invokes the capget syscall to check for necessary capabilities. Note that this depends
// on the binary having the capabilities set (i.e., via the `setcap` utility), and on VFS support.
// Alternatively, if the binary is executed as root, it automatically has all capabilities set.
func checkCaps() {
	caps := new(capsV3)
	caps.hdr.version = _LINUX_CAPABILITY_VERSION_3

	// Use RawSyscall since we do not expect it to block
	_, _, e1 := unix.RawSyscall(unix.SYS_CAPGET, uintptr(unsafe.Pointer(&caps.hdr)), uintptr(unsafe.Pointer(&caps.data)), 0)
	if e1 != 0 {
		fmt.Println("capget() failed:", e1.Error())
		return
	}

	if (caps.data[0].effective&CAP_SYS_RAWIO == 0) && (caps.data[0].effective&CAP_SYS_ADMIN == 0) {
		fmt.Println("Neither cap_sys_rawio nor cap_sys_admin are in effect. Device access will probably fail.")
	}
}

func main() {
	fmt.Println("Go nvme Reference Implementation")
	fmt.Printf("Built with %s on %s (%s)\n\n", runtime.Version(), runtime.GOOS, runtime.GOARCH)

	device := flag.String("device", "", "NVMe device from which to read SMART attributes, e.g. /dev/nvme0")
	flag.Parse()

	checkCaps()

	if *device == "" {
		flag.PrintDefaults()
		os.Exit(1)
	}

	d := nvme.NewNVMeDevice(*device)
	if err := d.Open(); err != nil {
		fmt.Fprintln(os.Stderr, "Cannot open NVMe device:", err)
		os.Exit(1)
	}
	defer d.Close()

	d.IdentifyController(os.Stdout)
	d.IdentifyNamespace(os.Stdout, 1)
	d.PrintSMART(os.Stdout)
}
