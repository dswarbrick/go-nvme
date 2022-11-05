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

const (
	// cf. NVM Express Base Specification 2.0c , section 5: Admin Command Set
	NVME_ADMIN_GET_LOG_PAGE uint8 = 0x02
	NVME_ADMIN_IDENTIFY     uint8 = 0x06
)

// Defined in <linux/nvme_ioctl.h> (first 64 bytes refer to NVM Express Base Specification 2.0c,
// figure 88: Common Command Format - Admin and NVM Vendor Specific Commands)
type nvmePassthruCommand struct {
	opcode       uint8
	flags        uint8
	rsvd1        uint16
	nsid         uint32
	cdw2         uint32
	cdw3         uint32
	metadata     uint64
	addr         uint64
	metadata_len uint32
	data_len     uint32
	cdw10        uint32
	cdw11        uint32
	cdw12        uint32
	cdw13        uint32
	cdw14        uint32
	cdw15        uint32
	timeout_ms   uint32
	result       uint32
} // 72 bytes
