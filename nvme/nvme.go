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

package nvme

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"math/big"
	"unsafe"

	"github.com/dswarbrick/go-nvme/ioctl"

	"golang.org/x/sys/unix"
)

var (
	// Defined in <linux/nvme_ioctl.h>
	NVME_IOCTL_ADMIN_CMD = ioctl.Iowr('N', 0x41, unsafe.Sizeof(nvmePassthruCommand{}))
)

type NVMeDevice struct {
	Name string
	fd   int
}

func NewNVMeDevice(name string) *NVMeDevice {
	return &NVMeDevice{name, -1}
}

func (d *NVMeDevice) Open() (err error) {
	d.fd, err = unix.Open(d.Name, unix.O_RDWR, 0600)
	return err
}

func (d *NVMeDevice) Close() error {
	return unix.Close(d.fd)
}

func (d *NVMeDevice) IdentifyController(w io.Writer) (NVMeController, error) {
	var buf [4096]byte

	cmd := nvmePassthruCommand{
		opcode:   NVME_ADMIN_IDENTIFY,
		nsid:     0, // Namespace 0, since we are identifying the controller
		addr:     uint64(uintptr(unsafe.Pointer(&buf[0]))),
		data_len: uint32(len(buf)),
		cdw10:    1, // Identify controller
	}

	if err := ioctl.Ioctl(uintptr(d.fd), NVME_IOCTL_ADMIN_CMD, uintptr(unsafe.Pointer(&cmd))); err != nil {
		return NVMeController{}, err
	}

	fmt.Fprintf(w, "NVMe call: opcode=%#02x, size=%#04x, nsid=%#08x, cdw10=%#08x\n",
		cmd.opcode, cmd.data_len, cmd.nsid, cmd.cdw10)

	var idCtrlr nvmeIdentController

	binary.Read(bytes.NewBuffer(buf[:]), NativeEndian, &idCtrlr)

	controller := NVMeController{
		VendorID:        idCtrlr.VendorID,
		ModelNumber:     string(idCtrlr.ModelNumber[:]),
		SerialNumber:    string(bytes.TrimSpace(idCtrlr.SerialNumber[:])),
		FirmwareVersion: string(idCtrlr.Firmware[:]),
		MaxDataXferSize: 1 << idCtrlr.Mdts,
		// Convert IEEE OUI ID from big-endian
		OUI: uint32(idCtrlr.IEEE[0]) | uint32(idCtrlr.IEEE[1])<<8 | uint32(idCtrlr.IEEE[2])<<16,
	}

	fmt.Fprintln(w)
	controller.Print(w)

	for _, ps := range idCtrlr.Psd {
		if ps.MaxPower > 0 {
			fmt.Fprintf(w, "%+v\n", ps)
		}
	}

	return controller, nil
}

func (d *NVMeDevice) IdentifyNamespace(w io.Writer, namespace uint32) error {
	var buf [4096]byte

	cmd := nvmePassthruCommand{
		opcode:   NVME_ADMIN_IDENTIFY,
		nsid:     namespace,
		addr:     uint64(uintptr(unsafe.Pointer(&buf[0]))),
		data_len: uint32(len(buf)),
		cdw10:    0,
	}

	if err := ioctl.Ioctl(uintptr(d.fd), NVME_IOCTL_ADMIN_CMD, uintptr(unsafe.Pointer(&cmd))); err != nil {
		return err
	}

	fmt.Fprintf(w, "NVMe call: opcode=%#02x, size=%#04x, nsid=%#08x, cdw10=%#08x\n",
		cmd.opcode, cmd.data_len, cmd.nsid, cmd.cdw10)

	var ns nvmeIdentNamespace

	binary.Read(bytes.NewBuffer(buf[:]), NativeEndian, &ns)

	fmt.Fprintf(w, "Namespace %d size: %d sectors\n", namespace, ns.Nsze)
	fmt.Fprintf(w, "Namespace %d utilisation: %d sectors\n", namespace, ns.Nuse)

	return nil
}

func (d *NVMeDevice) PrintSMART(w io.Writer) error {
	buf := make([]byte, 512)

	// Read SMART log
	if err := d.readLogPage(0x02, &buf); err != nil {
		return err
	}

	var sl nvmeSMARTLog

	binary.Read(bytes.NewBuffer(buf[:]), NativeEndian, &sl)

	unitsRead := le128ToBigInt(sl.DataUnitsRead)
	unitsWritten := le128ToBigInt(sl.DataUnitsWritten)
	unit := big.NewInt(512 * 1000)

	fmt.Fprintln(w, "\nSMART data follows:")
	fmt.Fprintf(w, "Critical warning: %#02x\n", sl.CritWarning)
	fmt.Fprintf(w, "Temperature: %d° Celsius\n",
		(uint16(sl.Temperature[0])|uint16(sl.Temperature[1])<<8)-273) // Kelvin to degrees Celsius
	fmt.Fprintf(w, "Avail. spare: %d%%\n", sl.AvailSpare)
	fmt.Fprintf(w, "Avail. spare threshold: %d%%\n", sl.SpareThresh)
	fmt.Fprintf(w, "Percentage used: %d%%\n", sl.PercentUsed)
	fmt.Fprintf(w, "Data units read: %d [%s]\n",
		unitsRead, formatBigBytes(new(big.Int).Mul(unitsRead, unit)))
	fmt.Fprintf(w, "Data units written: %d [%s]\n",
		unitsWritten, formatBigBytes(new(big.Int).Mul(unitsWritten, unit)))
	fmt.Fprintf(w, "Host read commands: %d\n", le128ToBigInt(sl.HostReads))
	fmt.Fprintf(w, "Host write commands: %d\n", le128ToBigInt(sl.HostWrites))
	fmt.Fprintf(w, "Controller busy time: %d\n", le128ToBigInt(sl.CtrlBusyTime))
	fmt.Fprintf(w, "Power cycles: %d\n", le128ToBigInt(sl.PowerCycles))
	fmt.Fprintf(w, "Power on hours: %d\n", le128ToBigInt(sl.PowerOnHours))
	fmt.Fprintf(w, "Unsafe shutdowns: %d\n", le128ToBigInt(sl.UnsafeShutdowns))
	fmt.Fprintf(w, "Media & data integrity errors: %d\n", le128ToBigInt(sl.MediaErrors))
	fmt.Fprintf(w, "Error information log entries: %d\n", le128ToBigInt(sl.NumErrLogEntries))

	return nil
}

func (d *NVMeDevice) readLogPage(logID uint8, buf *[]byte) error {
	bufLen := len(*buf)

	if (bufLen < 4) || (bufLen > 0x4000) || (bufLen%4 != 0) {
		return fmt.Errorf("invalid buffer size")
	}

	cmd := nvmePassthruCommand{
		opcode:   NVME_ADMIN_GET_LOG_PAGE,
		nsid:     0xffffffff, // FIXME
		addr:     uint64(uintptr(unsafe.Pointer(&(*buf)[0]))),
		data_len: uint32(bufLen),
		cdw10:    uint32(logID) | (((uint32(bufLen) / 4) - 1) << 16),
	}

	return ioctl.Ioctl(uintptr(d.fd), NVME_IOCTL_ADMIN_CMD, uintptr(unsafe.Pointer(&cmd)))
}

type nvmeIdentPowerState struct {
	MaxPower        uint16 // Centiwatts
	Rsvd2           uint8
	Flags           uint8
	EntryLat        uint32 // Microseconds
	ExitLat         uint32 // Microseconds
	ReadTput        uint8
	ReadLat         uint8
	WriteTput       uint8
	WriteLat        uint8
	IdlePower       uint16
	IdleScale       uint8
	Rsvd19          uint8
	ActivePower     uint16
	ActiveWorkScale uint8
	Rsvd23          [9]byte
}

type nvmeLBAF struct {
	Ms uint16
	Ds uint8
	Rp uint8
}

type nvmeIdentNamespace struct {
	Nsze    uint64
	Ncap    uint64
	Nuse    uint64
	Nsfeat  uint8
	Nlbaf   uint8
	Flbas   uint8
	Mc      uint8
	Dpc     uint8
	Dps     uint8
	Nmic    uint8
	Rescap  uint8
	Fpi     uint8
	Rsvd33  uint8
	Nawun   uint16
	Nawupf  uint16
	Nacwu   uint16
	Nabsn   uint16
	Nabo    uint16
	Nabspf  uint16
	Rsvd46  [2]byte
	Nvmcap  [16]byte
	Rsvd64  [40]byte
	Nguid   [16]byte
	EUI64   [8]byte
	Lbaf    [16]nvmeLBAF
	Rsvd192 [192]byte
	Vs      [3712]byte
} // 4096 bytes

type nvmeSMARTLog struct {
	CritWarning      uint8
	Temperature      [2]uint8
	AvailSpare       uint8
	SpareThresh      uint8
	PercentUsed      uint8
	Rsvd6            [26]byte
	DataUnitsRead    [16]byte
	DataUnitsWritten [16]byte
	HostReads        [16]byte
	HostWrites       [16]byte
	CtrlBusyTime     [16]byte
	PowerCycles      [16]byte
	PowerOnHours     [16]byte
	UnsafeShutdowns  [16]byte
	MediaErrors      [16]byte
	NumErrLogEntries [16]byte
	WarningTempTime  uint32
	CritCompTime     uint32
	TempSensor       [8]uint16
	Rsvd216          [296]byte
} // 512 bytes
