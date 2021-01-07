// Copyright 2014 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// +build aix darwin dragonfly freebsd linux netbsd openbsd solaris windows

package icmp

import (
	"net"
	"os"
	"runtime"
	"syscall"

	"golang.org/x/net/ipv4"
	"golang.org/x/net/ipv6"
)

const sysIP_STRIPHDR = 0x17 // for now only darwin supports this option

// ListenPacket listens for incoming ICMP packets addressed to
// address. See net.Dial for the syntax of address.
//
// For non-privileged datagram-oriented ICMP endpoints, network must
// be "udp4" or "udp6". The endpoint allows to read, write a few
// limited ICMP messages such as echo request and echo reply.
// Currently only Darwin and Linux support this.
//
// Examples:
//	ListenPacket("udp4", "192.168.0.1")
//	ListenPacket("udp4", "0.0.0.0")
//	ListenPacket("udp6", "fe80::1%en0")
//	ListenPacket("udp6", "::")
//
// For privileged raw ICMP endpoints, network must be "ip4" or "ip6"
// followed by a colon and an ICMP protocol number or name.
//
// Examples:
//	ListenPacket("ip4:icmp", "192.168.0.1")
//	ListenPacket("ip4:1", "0.0.0.0")
//	ListenPacket("ip6:ipv6-icmp", "fe80::1%en0")
//	ListenPacket("ip6:58", "::")
func ListenPacket(network, address string) (*PacketConn, error) {
	var family, proto int
	switch network {
	case "udp4":
		family, proto = syscall.AF_INET, ProtocolICMP
	case "udp6":
		family, proto = syscall.AF_INET6, ProtocolIPv6ICMP
	default:
		i := last(network, ':')
		if i < 0 {
			i = len(network)
		}
		switch network[:i] {
		case "ip4":
			proto = ProtocolICMP
		case "ip6":
			proto = ProtocolIPv6ICMP
		}
	}
	var cerr error
	var c net.PacketConn
	switch family {
	case syscall.AF_INET, syscall.AF_INET6:
		s, err := syscall.Socket(family, syscall.SOCK_DGRAM, proto)
		if err != nil {
			return nil, os.NewSyscallError("socket", err)
		}
		if runtime.GOOS == "darwin" && family == syscall.AF_INET {
			if err := syscall.SetsockoptInt(s, 0, sysIP_STRIPHDR, 1); err != nil {
				syscall.Close(s)
				return nil, os.NewSyscallError("setsockopt", err)
			}
		}
		sa, err := sockaddr(family, address)
		if err != nil {
			syscall.Close(s)
			return nil, err
		}
		if err := syscall.Bind(s, sa); err != nil {
			syscall.Close(s)
			return nil, os.NewSyscallError("bind", err)
		}
		f := os.NewFile(uintptr(s), "datagram-oriented icmp")
		c, cerr = net.FilePacketConn(f)
		f.Close()
	default:
		c, cerr = net.ListenPacket(network, address)
	}
	if cerr != nil {
		return nil, cerr
	}
	switch proto {
	case ProtocolICMP:
		return &PacketConn{c: c, p4: ipv4.NewPacketConn(c)}, nil
	case ProtocolIPv6ICMP:
		return &PacketConn{c: c, p6: ipv6.NewPacketConn(c)}, nil
	default:
		return &PacketConn{c: c}, nil
	}
}

const (
	ProtocolIP             = 0   // IPv4 encapsulation, pseudo protocol number
	ProtocolHOPOPT         = 0   // IPv6 Hop-by-Hop Option
	ProtocolICMP           = 1   // Internet Control Message
	ProtocolIGMP           = 2   // Internet Group Management
	ProtocolGGP            = 3   // Gateway-to-Gateway
	ProtocolIPv4           = 4   // IPv4 encapsulation
	ProtocolST             = 5   // Stream
	ProtocolTCP            = 6   // Transmission Control
	ProtocolCBT            = 7   // CBT
	ProtocolEGP            = 8   // Exterior Gateway Protocol
	ProtocolIGP            = 9   // any private interior gateway (used by Cisco for their IGRP)
	ProtocolBBNRCCMON      = 10  // BBN RCC Monitoring
	ProtocolNVPII          = 11  // Network Voice Protocol
	ProtocolPUP            = 12  // PUP
	ProtocolEMCON          = 14  // EMCON
	ProtocolXNET           = 15  // Cross Net Debugger
	ProtocolCHAOS          = 16  // Chaos
	ProtocolUDP            = 17  // User Datagram
	ProtocolMUX            = 18  // Multiplexing
	ProtocolDCNMEAS        = 19  // DCN Measurement Subsystems
	ProtocolHMP            = 20  // Host Monitoring
	ProtocolPRM            = 21  // Packet Radio Measurement
	ProtocolXNSIDP         = 22  // XEROX NS IDP
	ProtocolTRUNK1         = 23  // Trunk-1
	ProtocolTRUNK2         = 24  // Trunk-2
	ProtocolLEAF1          = 25  // Leaf-1
	ProtocolLEAF2          = 26  // Leaf-2
	ProtocolRDP            = 27  // Reliable Data Protocol
	ProtocolIRTP           = 28  // Internet Reliable Transaction
	ProtocolISOTP4         = 29  // ISO Transport Protocol Class 4
	ProtocolNETBLT         = 30  // Bulk Data Transfer Protocol
	ProtocolMFENSP         = 31  // MFE Network Services Protocol
	ProtocolMERITINP       = 32  // MERIT Internodal Protocol
	ProtocolDCCP           = 33  // Datagram Congestion Control Protocol
	Protocol3PC            = 34  // Third Party Connect Protocol
	ProtocolIDPR           = 35  // Inter-Domain Policy Routing Protocol
	ProtocolXTP            = 36  // XTP
	ProtocolDDP            = 37  // Datagram Delivery Protocol
	ProtocolIDPRCMTP       = 38  // IDPR Control Message Transport Proto
	ProtocolTPPP           = 39  // TP++ Transport Protocol
	ProtocolIL             = 40  // IL Transport Protocol
	ProtocolIPv6           = 41  // IPv6 encapsulation
	ProtocolSDRP           = 42  // Source Demand Routing Protocol
	ProtocolIPv6Route      = 43  // Routing Header for IPv6
	ProtocolIPv6Frag       = 44  // Fragment Header for IPv6
	ProtocolIDRP           = 45  // Inter-Domain Routing Protocol
	ProtocolRSVP           = 46  // Reservation Protocol
	ProtocolGRE            = 47  // Generic Routing Encapsulation
	ProtocolDSR            = 48  // Dynamic Source Routing Protocol
	ProtocolBNA            = 49  // BNA
	ProtocolESP            = 50  // Encap Security Payload
	ProtocolAH             = 51  // Authentication Header
	ProtocolINLSP          = 52  // Integrated Net Layer Security  TUBA
	ProtocolNARP           = 54  // NBMA Address Resolution Protocol
	ProtocolMOBILE         = 55  // IP Mobility
	ProtocolTLSP           = 56  // Transport Layer Security Protocol using Kryptonet key management
	ProtocolSKIP           = 57  // SKIP
	ProtocolIPv6ICMP       = 58  // ICMP for IPv6
	ProtocolIPv6NoNxt      = 59  // No Next Header for IPv6
	ProtocolIPv6Opts       = 60  // Destination Options for IPv6
	ProtocolCFTP           = 62  // CFTP
	ProtocolSATEXPAK       = 64  // SATNET and Backroom EXPAK
	ProtocolKRYPTOLAN      = 65  // Kryptolan
	ProtocolRVD            = 66  // MIT Remote Virtual Disk Protocol
	ProtocolIPPC           = 67  // Internet Pluribus Packet Core
	ProtocolSATMON         = 69  // SATNET Monitoring
	ProtocolVISA           = 70  // VISA Protocol
	ProtocolIPCV           = 71  // Internet Packet Core Utility
	ProtocolCPNX           = 72  // Computer Protocol Network Executive
	ProtocolCPHB           = 73  // Computer Protocol Heart Beat
	ProtocolWSN            = 74  // Wang Span Network
	ProtocolPVP            = 75  // Packet Video Protocol
	ProtocolBRSATMON       = 76  // Backroom SATNET Monitoring
	ProtocolSUNND          = 77  // SUN ND PROTOCOL-Temporary
	ProtocolWBMON          = 78  // WIDEBAND Monitoring
	ProtocolWBEXPAK        = 79  // WIDEBAND EXPAK
	ProtocolISOIP          = 80  // ISO Internet Protocol
	ProtocolVMTP           = 81  // VMTP
	ProtocolSECUREVMTP     = 82  // SECURE-VMTP
	ProtocolVINES          = 83  // VINES
	ProtocolTTP            = 84  // Transaction Transport Protocol
	ProtocolIPTM           = 84  // Internet Protocol Traffic Manager
	ProtocolNSFNETIGP      = 85  // NSFNET-IGP
	ProtocolDGP            = 86  // Dissimilar Gateway Protocol
	ProtocolTCF            = 87  // TCF
	ProtocolEIGRP          = 88  // EIGRP
	ProtocolOSPFIGP        = 89  // OSPFIGP
	ProtocolSpriteRPC      = 90  // Sprite RPC Protocol
	ProtocolLARP           = 91  // Locus Address Resolution Protocol
	ProtocolMTP            = 92  // Multicast Transport Protocol
	ProtocolAX25           = 93  // AX.25 Frames
	ProtocolIPIP           = 94  // IP-within-IP Encapsulation Protocol
	ProtocolSCCSP          = 96  // Semaphore Communications Sec. Pro.
	ProtocolETHERIP        = 97  // Ethernet-within-IP Encapsulation
	ProtocolENCAP          = 98  // Encapsulation Header
	ProtocolGMTP           = 100 // GMTP
	ProtocolIFMP           = 101 // Ipsilon Flow Management Protocol
	ProtocolPNNI           = 102 // PNNI over IP
	ProtocolPIM            = 103 // Protocol Independent Multicast
	ProtocolARIS           = 104 // ARIS
	ProtocolSCPS           = 105 // SCPS
	ProtocolQNX            = 106 // QNX
	ProtocolAN             = 107 // Active Networks
	ProtocolIPComp         = 108 // IP Payload Compression Protocol
	ProtocolSNP            = 109 // Sitara Networks Protocol
	ProtocolCompaqPeer     = 110 // Compaq Peer Protocol
	ProtocolIPXinIP        = 111 // IPX in IP
	ProtocolVRRP           = 112 // Virtual Router Redundancy Protocol
	ProtocolPGM            = 113 // PGM Reliable Transport Protocol
	ProtocolL2TP           = 115 // Layer Two Tunneling Protocol
	ProtocolDDX            = 116 // D-II Data Exchange (DDX)
	ProtocolIATP           = 117 // Interactive Agent Transfer Protocol
	ProtocolSTP            = 118 // Schedule Transfer Protocol
	ProtocolSRP            = 119 // SpectraLink Radio Protocol
	ProtocolUTI            = 120 // UTI
	ProtocolSMP            = 121 // Simple Message Protocol
	ProtocolPTP            = 123 // Performance Transparency Protocol
	ProtocolISIS           = 124 // ISIS over IPv4
	ProtocolFIRE           = 125 // FIRE
	ProtocolCRTP           = 126 // Combat Radio Transport Protocol
	ProtocolCRUDP          = 127 // Combat Radio User Datagram
	ProtocolSSCOPMCE       = 128 // SSCOPMCE
	ProtocolIPLT           = 129 // IPLT
	ProtocolSPS            = 130 // Secure Packet Shield
	ProtocolPIPE           = 131 // Private IP Encapsulation within IP
	ProtocolSCTP           = 132 // Stream Control Transmission Protocol
	ProtocolFC             = 133 // Fibre Channel
	ProtocolRSVPE2EIGNORE  = 134 // RSVP-E2E-IGNORE
	ProtocolMobilityHeader = 135 // Mobility Header
	ProtocolUDPLite        = 136 // UDPLite
	ProtocolMPLSinIP       = 137 // MPLS-in-IP
	ProtocolMANET          = 138 // MANET Protocols
	ProtocolHIP            = 139 // Host Identity Protocol
	ProtocolShim6          = 140 // Shim6 Protocol
	ProtocolWESP           = 141 // Wrapped Encapsulating Security Payload
	ProtocolROHC           = 142 // Robust Header Compression
	ProtocolReserved       = 255 // Reserved
)

const (
	AddrFamilyIPv4                          = 1     // IP (IP version 4)
	AddrFamilyIPv6                          = 2     // IP6 (IP version 6)
	AddrFamilyNSAP                          = 3     // NSAP
	AddrFamilyHDLC                          = 4     // HDLC (8-bit multidrop)
	AddrFamilyBBN1822                       = 5     // BBN 1822
	AddrFamily802                           = 6     // 802 (includes all 802 media plus Ethernet "canonical format")
	AddrFamilyE163                          = 7     // E.163
	AddrFamilyE164                          = 8     // E.164 (SMDS, Frame Relay, ATM)
	AddrFamilyF69                           = 9     // F.69 (Telex)
	AddrFamilyX121                          = 10    // X.121 (X.25, Frame Relay)
	AddrFamilyIPX                           = 11    // IPX
	AddrFamilyAppletalk                     = 12    // Appletalk
	AddrFamilyDecnetIV                      = 13    // Decnet IV
	AddrFamilyBanyanVines                   = 14    // Banyan Vines
	AddrFamilyE164withSubaddress            = 15    // E.164 with NSAP format subaddress
	AddrFamilyDNS                           = 16    // DNS (Domain Name System)
	AddrFamilyDistinguishedName             = 17    // Distinguished Name
	AddrFamilyASNumber                      = 18    // AS Number
	AddrFamilyXTPoverIPv4                   = 19    // XTP over IP version 4
	AddrFamilyXTPoverIPv6                   = 20    // XTP over IP version 6
	AddrFamilyXTPnativemodeXTP              = 21    // XTP native mode XTP
	AddrFamilyFibreChannelWorldWidePortName = 22    // Fibre Channel World-Wide Port Name
	AddrFamilyFibreChannelWorldWideNodeName = 23    // Fibre Channel World-Wide Node Name
	AddrFamilyGWID                          = 24    // GWID
	AddrFamilyL2VPN                         = 25    // AFI for L2VPN information
	AddrFamilyMPLSTPSectionEndpointID       = 26    // MPLS-TP Section Endpoint Identifier
	AddrFamilyMPLSTPLSPEndpointID           = 27    // MPLS-TP LSP Endpoint Identifier
	AddrFamilyMPLSTPPseudowireEndpointID    = 28    // MPLS-TP Pseudowire Endpoint Identifier
	AddrFamilyMTIPv4                        = 29    // MT IP: Multi-Topology IP version 4
	AddrFamilyMTIPv6                        = 30    // MT IPv6: Multi-Topology IP version 6
	AddrFamilyEIGRPCommonServiceFamily      = 16384 // EIGRP Common Service Family
	AddrFamilyEIGRPIPv4ServiceFamily        = 16385 // EIGRP IPv4 Service Family
	AddrFamilyEIGRPIPv6ServiceFamily        = 16386 // EIGRP IPv6 Service Family
	AddrFamilyLISPCanonicalAddressFormat    = 16387 // LISP Canonical Address Format (LCAF)
	AddrFamilyBGPLS                         = 16388 // BGP-LS
	AddrFamily48bitMAC                      = 16389 // 48-bit MAC
	AddrFamily64bitMAC                      = 16390 // 64-bit MAC
	AddrFamilyOUI                           = 16391 // OUI
	AddrFamilyMACFinal24bits                = 16392 // MAC/24
	AddrFamilyMACFinal40bits                = 16393 // MAC/40
	AddrFamilyIPv6Initial64bits             = 16394 // IPv6/64
	AddrFamilyRBridgePortID                 = 16395 // RBridge Port ID
	AddrFamilyTRILLNickname                 = 16396 // TRILL Nickname
)
