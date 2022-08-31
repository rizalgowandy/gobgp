package bgp

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"net/netip"
)

// MUPExtended represents BGP MUP Extended Community as described in
// https://datatracker.ietf.org/doc/html/draft-mpmz-bess-mup-safi-00#section-3.2
type MUPExtended struct {
	SubType    ExtendedCommunityAttrSubType
	SegmentID2 uint16
	SegmentID4 uint32
}

func (e *MUPExtended) Serialize() ([]byte, error) {
	buf := make([]byte, 8)
	buf[0] = byte(EC_TYPE_MUP)
	buf[1] = byte(EC_SUBTYPE_MUP_DIRECT_SEG)
	binary.BigEndian.PutUint16(buf[2:4], e.SegmentID2)
	binary.BigEndian.PutUint32(buf[4:8], e.SegmentID4)
	return buf, nil
}

func (e *MUPExtended) String() string {
	return fmt.Sprintf("%d:%d", e.SegmentID2, e.SegmentID4)
}

func (e *MUPExtended) MarshalJSON() ([]byte, error) {
	t, s := e.GetTypes()
	return json.Marshal(struct {
		Type      ExtendedCommunityAttrType    `json:"type"`
		Subtype   ExtendedCommunityAttrSubType `json:"subtype"`
		SegmentID string                       `json:"segmend_id"`
	}{
		Type:      t,
		Subtype:   s,
		SegmentID: fmt.Sprintf("%d:%d", e.SegmentID2, e.SegmentID4),
	})
}

func (e *MUPExtended) GetTypes() (ExtendedCommunityAttrType, ExtendedCommunityAttrSubType) {
	return EC_TYPE_MUP, EC_SUBTYPE_MUP_DIRECT_SEG
}

func (e *MUPExtended) Flat() map[string]string {
	return map[string]string{}
}

func NewMUPExtended(sid2 uint16, sid4 uint32) *MUPExtended {
	return &MUPExtended{
		SubType:    EC_SUBTYPE_MUP_DIRECT_SEG,
		SegmentID2: sid2,
		SegmentID4: sid4,
	}
}

func parseMUPExtended(data []byte) (ExtendedCommunityInterface, error) {
	typ := ExtendedCommunityAttrType(data[0])
	if typ != EC_TYPE_MUP {
		return nil, NewMessageError(BGP_ERROR_UPDATE_MESSAGE_ERROR, BGP_ERROR_SUB_MALFORMED_ATTRIBUTE_LIST, nil, fmt.Sprintf("ext comm type is not EC_TYPE_MUP: %d", data[0]))
	}
	subType := ExtendedCommunityAttrSubType(data[1])
	if subType == EC_SUBTYPE_MUP_DIRECT_SEG {
		sid2 := binary.BigEndian.Uint16(data[2:4])
		sid4 := binary.BigEndian.Uint32(data[4:8])
		return NewMUPExtended(sid2, sid4), nil
	}
	return nil, NewMessageError(BGP_ERROR_UPDATE_MESSAGE_ERROR, BGP_ERROR_SUB_MALFORMED_ATTRIBUTE_LIST, nil, fmt.Sprintf("unknown mup subtype: %d", subType))
}

// BGP MUP SAFI Architecture Type as described in
// https://datatracker.ietf.org/doc/html/draft-mpmz-bess-mup-safi-00#section-3.1
const (
	MUP_ARCH_TYPE_UNDEFINED = iota
	MUP_ARCH_TYPE_3GPP_5G
)

// BGP MUP SAFI Route Type as described in
// https://datatracker.ietf.org/doc/html/draft-mpmz-bess-mup-safi-00#section-3.1
const (
	_ = iota
	MUP_ROUTE_TYPE_INTERWORK_SEGMENT_DISCOVERY
	MUP_ROUTE_TYPE_DIRECT_SEGMENT_DISCOVERY
	MUP_ROUTE_TYPE_TYPE_1_SESSION_TRANSFORMED
	MUP_ROUTE_TYPE_TYPE_2_SESSION_TRANSFORMED
)

type MUPRouteTypeInterface interface {
	DecodeFromBytes([]byte) error
	Serialize() ([]byte, error)
	AFI() uint16
	Len() int
	String() string
	MarshalJSON() ([]byte, error)
	rd() RouteDistinguisherInterface
}

func getMUPRouteType(at uint8, rt uint16) (MUPRouteTypeInterface, error) {
	switch rt {
	case MUP_ROUTE_TYPE_INTERWORK_SEGMENT_DISCOVERY:
		if at == MUP_ARCH_TYPE_3GPP_5G {
			return &MUPInterworkSegmentDiscoveryRoute{}, nil
		}
	case MUP_ROUTE_TYPE_DIRECT_SEGMENT_DISCOVERY:
		if at == MUP_ARCH_TYPE_3GPP_5G {
			return &MUPDirectSegmentDiscoveryRoute{}, nil
		}
	case MUP_ROUTE_TYPE_TYPE_1_SESSION_TRANSFORMED:
		if at == MUP_ARCH_TYPE_3GPP_5G {
			return &MUPType1SessionTransformedRoute{}, nil
		}
	case MUP_ROUTE_TYPE_TYPE_2_SESSION_TRANSFORMED:
		if at == MUP_ARCH_TYPE_3GPP_5G {
			return &MUPType2SessionTransformedRoute{}, nil
		}
	}
	return nil, NewMessageError(BGP_ERROR_UPDATE_MESSAGE_ERROR, BGP_ERROR_SUB_MALFORMED_ATTRIBUTE_LIST, nil, fmt.Sprintf("Unknown MUP Architecture and Route type: %d, %d", at, rt))
}

type MUPNLRI struct {
	PrefixDefault
	ArchitectureType uint8
	RouteType        uint16
	Length           uint8
	RouteTypeData    MUPRouteTypeInterface
}

func (n *MUPNLRI) DecodeFromBytes(data []byte, options ...*MarshallingOption) error {
	if len(data) < 4 {
		return NewMessageError(BGP_ERROR_UPDATE_MESSAGE_ERROR, BGP_ERROR_SUB_MALFORMED_ATTRIBUTE_LIST, nil, "Not all MUPNLRI bytes available")
	}
	n.ArchitectureType = data[0]
	n.RouteType = binary.BigEndian.Uint16(data[1:3])
	n.Length = data[3]
	data = data[4:]
	if len(data) < int(n.Length) {
		return NewMessageError(BGP_ERROR_UPDATE_MESSAGE_ERROR, BGP_ERROR_SUB_MALFORMED_ATTRIBUTE_LIST, nil, "Not all MUPNLRI Route type bytes available")
	}
	r, err := getMUPRouteType(n.ArchitectureType, n.RouteType)
	if err != nil {
		return err
	}
	n.RouteTypeData = r
	return n.RouteTypeData.DecodeFromBytes(data[:n.Length])
}

func (n *MUPNLRI) Serialize(options ...*MarshallingOption) ([]byte, error) {
	buf := make([]byte, 4)
	buf[0] = n.ArchitectureType
	binary.BigEndian.PutUint16(buf[1:3], n.RouteType)
	buf[3] = n.Length
	tbuf, err := n.RouteTypeData.Serialize()
	if err != nil {
		return nil, err
	}
	return append(buf, tbuf...), nil
}

func (n *MUPNLRI) AFI() uint16 {
	return n.RouteTypeData.AFI()
}

func (n *MUPNLRI) SAFI() uint8 {
	return SAFI_MUP
}

func (n *MUPNLRI) Len(options ...*MarshallingOption) int {
	return int(n.Length) + 4
}

func (n *MUPNLRI) String() string {
	if n.RouteTypeData != nil {
		return n.RouteTypeData.String()
	}
	return fmt.Sprintf("%d:%d:%d", n.ArchitectureType, n.RouteType, n.Length)
}

func (n *MUPNLRI) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		ArchitectureType uint8                 `json:"arch_type"`
		RouteType        uint16                `json:"route_type"`
		Value            MUPRouteTypeInterface `json:"value"`
	}{
		ArchitectureType: n.ArchitectureType,
		RouteType:        n.RouteType,
		Value:            n.RouteTypeData,
	})
}

func (n *MUPNLRI) RD() RouteDistinguisherInterface {
	return n.RouteTypeData.rd()
}

func (l *MUPNLRI) Flat() map[string]string {
	return map[string]string{}
}

func NewMUPNLRI(at uint8, rt uint16, data MUPRouteTypeInterface) *MUPNLRI {
	var l uint8
	if data != nil {
		l = uint8(data.Len())
	}
	return &MUPNLRI{
		ArchitectureType: at,
		RouteType:        rt,
		Length:           l,
		RouteTypeData:    data,
	}
}

// MUPInterworkSegmentDiscoveryRoute represents BGP Interwork Segment Discovery route as described in
// https://datatracker.ietf.org/doc/html/draft-mpmz-bess-mup-safi-00#section-3.1.1
type MUPInterworkSegmentDiscoveryRoute struct {
	RD           RouteDistinguisherInterface
	PrefixLength uint8
	Prefix       netip.Prefix
}

func NewMUPInterworkSegmentDiscoveryRoute(rd RouteDistinguisherInterface, prefix netip.Prefix) *MUPNLRI {
	return NewMUPNLRI(MUP_ARCH_TYPE_3GPP_5G, MUP_ROUTE_TYPE_INTERWORK_SEGMENT_DISCOVERY, &MUPInterworkSegmentDiscoveryRoute{
		RD:           rd,
		PrefixLength: uint8(prefix.Bits()),
		Prefix:       prefix,
	})
}

func (r *MUPInterworkSegmentDiscoveryRoute) DecodeFromBytes(data []byte) error {
	r.RD = GetRouteDistinguisher(data)
	p := r.RD.Len()
	if len(data) < p {
		return NewMessageError(BGP_ERROR_UPDATE_MESSAGE_ERROR, BGP_ERROR_SUB_MALFORMED_ATTRIBUTE_LIST, nil, "invalid Interwork Segment Discovery Route length")
	}
	r.PrefixLength = data[p]
	p += 1
	addr, ok := netip.AddrFromSlice(data[p:])
	if !ok {
		return NewMessageError(BGP_ERROR_UPDATE_MESSAGE_ERROR, BGP_ERROR_SUB_MALFORMED_ATTRIBUTE_LIST, nil, fmt.Sprintf("Invalid Prefix: %x", data[p:]))
	}
	r.Prefix = netip.PrefixFrom(addr, int(r.PrefixLength))
	if r.Prefix.Bits() == -1 {
		return NewMessageError(BGP_ERROR_UPDATE_MESSAGE_ERROR, BGP_ERROR_SUB_MALFORMED_ATTRIBUTE_LIST, nil, fmt.Sprintf("Invalid Prefix: %s", r.Prefix))
	}
	return nil
}

func (r *MUPInterworkSegmentDiscoveryRoute) Serialize() ([]byte, error) {
	var buf []byte
	var err error
	if r.RD != nil {
		buf, err = r.RD.Serialize()
		if err != nil {
			return nil, err
		}
	} else {
		buf = make([]byte, 8)
	}
	buf = append(buf, r.PrefixLength)
	buf = append(buf, r.Prefix.Addr().AsSlice()...)
	return buf, nil
}

func (r *MUPInterworkSegmentDiscoveryRoute) AFI() uint16 {
	if r.Prefix.Addr().Is6() {
		return AFI_IP6
	}
	return AFI_IP
}

func (r *MUPInterworkSegmentDiscoveryRoute) Len() int {
	// RD(8) + PrefixLength(1) + Prefix(4 or 16)
	return 9 + int(r.Prefix.Addr().BitLen()/8)
}

func (r *MUPInterworkSegmentDiscoveryRoute) String() string {
	return fmt.Sprintf("[type:isd][rd:%s][prefix:%s]", r.RD, r.Prefix)
}

func (r *MUPInterworkSegmentDiscoveryRoute) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		RD     RouteDistinguisherInterface `json:"rd"`
		Prefix string                      `json:"prefix"`
	}{
		RD:     r.RD,
		Prefix: r.Prefix.String(),
	})
}

func (r *MUPInterworkSegmentDiscoveryRoute) rd() RouteDistinguisherInterface {
	return r.RD
}

// MUPDirectSegmentDiscoveryRoute represents BGP Direct Segment Discovery route as described in
// https://datatracker.ietf.org/doc/html/draft-mpmz-bess-mup-safi-00#section-3.1.2
type MUPDirectSegmentDiscoveryRoute struct {
	RD      RouteDistinguisherInterface
	Address netip.Addr
}

func NewMUPDirectSegmentDiscoveryRoute(rd RouteDistinguisherInterface, address netip.Addr) *MUPNLRI {
	return NewMUPNLRI(MUP_ARCH_TYPE_3GPP_5G, MUP_ROUTE_TYPE_DIRECT_SEGMENT_DISCOVERY, &MUPDirectSegmentDiscoveryRoute{
		RD:      rd,
		Address: address,
	})
}

func (r *MUPDirectSegmentDiscoveryRoute) DecodeFromBytes(data []byte) error {
	r.RD = GetRouteDistinguisher(data)
	rdLen := r.RD.Len()
	if len(data) != 12 && len(data) != 24 {
		return NewMessageError(BGP_ERROR_UPDATE_MESSAGE_ERROR, BGP_ERROR_SUB_MALFORMED_ATTRIBUTE_LIST, nil, "invalid Direct Segment Discovery Route length")
	}
	if len(data) == 12 {
		address, ok := netip.AddrFromSlice(data[rdLen : rdLen+4])
		if !ok {
			return NewMessageError(BGP_ERROR_UPDATE_MESSAGE_ERROR, BGP_ERROR_SUB_MALFORMED_ATTRIBUTE_LIST, nil, fmt.Sprintf("Invalid Address: %s", data[rdLen:rdLen+4]))
		}
		r.Address = address
	} else if len(data) == 24 {
		address, ok := netip.AddrFromSlice(data[rdLen : rdLen+16])
		if !ok {
			return NewMessageError(BGP_ERROR_UPDATE_MESSAGE_ERROR, BGP_ERROR_SUB_MALFORMED_ATTRIBUTE_LIST, nil, fmt.Sprintf("Invalid Address: %d", data[rdLen:rdLen+16]))
		}
		r.Address = address
	}
	return nil
}

func (r *MUPDirectSegmentDiscoveryRoute) Serialize() ([]byte, error) {
	var buf []byte
	var err error
	if r.RD != nil {
		buf, err = r.RD.Serialize()
		if err != nil {
			return nil, err
		}
	} else {
		buf = make([]byte, 8)
	}
	buf = append(buf, r.Address.AsSlice()...)
	return buf, nil
}

func (r *MUPDirectSegmentDiscoveryRoute) AFI() uint16 {
	if r.Address.Is6() {
		return AFI_IP6
	}
	return AFI_IP
}

func (r *MUPDirectSegmentDiscoveryRoute) Len() int {
	// RD(8) + Address(4 or 16)
	return 8 + r.Address.BitLen()/8
}

func (r *MUPDirectSegmentDiscoveryRoute) String() string {
	return fmt.Sprintf("[type:dsd][rd:%s][prefix:%s]", r.RD, r.Address)
}

func (r *MUPDirectSegmentDiscoveryRoute) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		RD      RouteDistinguisherInterface `json:"rd"`
		Address string                      `json:"address"`
	}{
		RD:      r.RD,
		Address: r.Address.String(),
	})
}

func (r *MUPDirectSegmentDiscoveryRoute) rd() RouteDistinguisherInterface {
	return r.RD
}

// MUPType1SessionTransformedRoute3GPP5G represents 3GPP 5G specific Type 1 Session Transformed (ST) Route as described in
// https://datatracker.ietf.org/doc/html/draft-mpmz-bess-mup-safi-00#section-3.1.3
type MUPType1SessionTransformedRoute struct {
	RD                    RouteDistinguisherInterface
	PrefixLength          uint8
	Prefix                netip.Addr
	TEID                  uint32
	QFI                   uint8
	EndpointAddressLength uint8
	EndpointAddress       netip.Addr
}

func NewMUPType1SessionTransformedRoute(rd RouteDistinguisherInterface, prefix netip.Addr, teid uint32, qfi uint8, ea netip.Addr) *MUPNLRI {
	return NewMUPNLRI(MUP_ARCH_TYPE_3GPP_5G, MUP_ROUTE_TYPE_TYPE_1_SESSION_TRANSFORMED, &MUPType1SessionTransformedRoute{
		RD:                    rd,
		PrefixLength:          uint8(prefix.BitLen()),
		Prefix:                prefix,
		TEID:                  teid,
		QFI:                   qfi,
		EndpointAddressLength: uint8(ea.BitLen()),
		EndpointAddress:       ea,
	})
}

func (r *MUPType1SessionTransformedRoute) DecodeFromBytes(data []byte) error {
	r.RD = GetRouteDistinguisher(data)
	p := r.RD.Len()
	if len(data) < p {
		return NewMessageError(BGP_ERROR_UPDATE_MESSAGE_ERROR, BGP_ERROR_SUB_MALFORMED_ATTRIBUTE_LIST, nil, "invalid 3GPP 5G specific Type 1 Session Transformed Route length")
	}
	r.PrefixLength = data[p]
	p += 1
	if r.PrefixLength == 32 || r.PrefixLength == 128 {
		prefix, ok := netip.AddrFromSlice(data[p : p+int(r.PrefixLength/8)])
		if !ok {
			return NewMessageError(BGP_ERROR_UPDATE_MESSAGE_ERROR, BGP_ERROR_SUB_MALFORMED_ATTRIBUTE_LIST, nil, fmt.Sprintf("Invalid Prefix: %x", data[p:p+int(r.PrefixLength/8)]))
		}
		r.Prefix = prefix
	} else {
		return NewMessageError(BGP_ERROR_UPDATE_MESSAGE_ERROR, BGP_ERROR_SUB_MALFORMED_ATTRIBUTE_LIST, nil, fmt.Sprintf("Invalid Prefix length: %d", r.PrefixLength))
	}
	p += int(r.PrefixLength / 8)
	r.TEID = binary.BigEndian.Uint32(data[p : p+4])
	p += 4
	r.QFI = data[p]
	p += 1
	r.EndpointAddressLength = data[p]
	p += 1
	if r.EndpointAddressLength == 32 || r.EndpointAddressLength == 128 {
		ea, ok := netip.AddrFromSlice(data[p : p+int(r.EndpointAddressLength/8)])
		if !ok {
			return NewMessageError(BGP_ERROR_UPDATE_MESSAGE_ERROR, BGP_ERROR_SUB_MALFORMED_ATTRIBUTE_LIST, nil, fmt.Sprintf("Invalid Endpoint Address: %x", data[p:p+int(r.EndpointAddressLength/8)]))
		}
		r.EndpointAddress = ea
	} else {
		return NewMessageError(BGP_ERROR_UPDATE_MESSAGE_ERROR, BGP_ERROR_SUB_MALFORMED_ATTRIBUTE_LIST, nil, fmt.Sprintf("Invalid Endpoint Address length: %d", r.EndpointAddressLength))
	}
	return nil
}

func (r *MUPType1SessionTransformedRoute) Serialize() ([]byte, error) {
	var buf []byte
	var err error
	if r.RD != nil {
		buf, err = r.RD.Serialize()
		if err != nil {
			return nil, err
		}
	} else {
		buf = make([]byte, 8)
	}
	buf = append(buf, r.PrefixLength)
	buf = append(buf, r.Prefix.AsSlice()...)
	t := make([]byte, 4)
	binary.BigEndian.PutUint32(t, r.TEID)
	buf = append(buf, t...)
	buf = append(buf, r.QFI)
	buf = append(buf, r.EndpointAddressLength)
	buf = append(buf, r.EndpointAddress.AsSlice()...)
	return buf, nil
}

func (r *MUPType1SessionTransformedRoute) AFI() uint16 {
	if r.Prefix.Is6() {
		return AFI_IP6
	}
	return AFI_IP
}

func (r *MUPType1SessionTransformedRoute) Len() int {
	// RD(8) + PrefixLength(1) + Prefix(4 or 16)
	// + TEID(4) + QFI(1) + EndpointAddressLength(1) + EndpointAddress(4 or 16)
	return 15 + int(r.PrefixLength/8) + int(r.EndpointAddressLength/8)
}

func (r *MUPType1SessionTransformedRoute) String() string {
	return fmt.Sprintf("[type:t1st][rd:%s][prefix:%s][teid:%d][qfi:%d][endpoint:%s]", r.RD, r.Prefix, r.TEID, r.QFI, r.EndpointAddress)
}

func (r *MUPType1SessionTransformedRoute) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		RD              RouteDistinguisherInterface `json:"rd"`
		Prefix          string                      `json:"prefix"`
		TEID            uint32                      `json:"teid"`
		QFI             uint8                       `json:"qfi"`
		EndpointAddress string                      `json:"endpoint_address"`
	}{
		RD:              r.RD,
		Prefix:          r.Prefix.String(),
		TEID:            r.TEID,
		QFI:             r.QFI,
		EndpointAddress: r.EndpointAddress.String(),
	})
}

func (r *MUPType1SessionTransformedRoute) rd() RouteDistinguisherInterface {
	return r.RD
}

// MUPType2SessionTransformedRoute represents 3GPP 5G specific Type 2 Session Transformed (ST) Route as described in
// https://datatracker.ietf.org/doc/html/draft-mpmz-bess-mup-safi-00#section-3.1.4
type MUPType2SessionTransformedRoute struct {
	RD                    RouteDistinguisherInterface
	EndpointAddressLength uint8
	EndpointAddress       netip.Addr
	TEID                  uint32
}

func NewMUPType2SessionTransformedRoute(rd RouteDistinguisherInterface, ea netip.Addr, teid uint32) *MUPNLRI {
	return NewMUPNLRI(MUP_ARCH_TYPE_3GPP_5G, MUP_ROUTE_TYPE_TYPE_2_SESSION_TRANSFORMED, &MUPType2SessionTransformedRoute{
		RD:                    rd,
		EndpointAddressLength: uint8(ea.BitLen()) + 32,
		EndpointAddress:       ea,
		TEID:                  teid,
	})
}

func (r *MUPType2SessionTransformedRoute) DecodeFromBytes(data []byte) error {
	r.RD = GetRouteDistinguisher(data)
	p := r.RD.Len()
	if len(data) < p {
		return NewMessageError(BGP_ERROR_UPDATE_MESSAGE_ERROR, BGP_ERROR_SUB_MALFORMED_ATTRIBUTE_LIST, nil, "invalid 3GPP 5G specific Type 2 Session Transformed Route length")
	}
	r.EndpointAddressLength = data[p]
	p += 1
	var ea netip.Addr
	var ok bool
	teidLen := 0
	if r.EndpointAddressLength >= 32 && r.EndpointAddressLength <= 64 {
		ea, ok = netip.AddrFromSlice(data[p : p+4])
		if !ok {
			return NewMessageError(BGP_ERROR_UPDATE_MESSAGE_ERROR, BGP_ERROR_SUB_MALFORMED_ATTRIBUTE_LIST, nil, fmt.Sprintf("Invalid Endpoint Address: %x", data[p:p+int(r.EndpointAddressLength/8)]))
		}
		p += 4
		teidLen = int(r.EndpointAddressLength)/8 - 4
	} else if r.EndpointAddressLength >= 128 && r.EndpointAddressLength <= 160 {
		ea, ok = netip.AddrFromSlice(data[p : p+16])
		if !ok {
			return NewMessageError(BGP_ERROR_UPDATE_MESSAGE_ERROR, BGP_ERROR_SUB_MALFORMED_ATTRIBUTE_LIST, nil, fmt.Sprintf("Invalid Endpoint Address: %x", data[p:p+int(r.EndpointAddressLength/8)]))
		}
		p += 16
		teidLen = int(r.EndpointAddressLength)/8 - 16
	} else {
		return NewMessageError(BGP_ERROR_UPDATE_MESSAGE_ERROR, BGP_ERROR_SUB_MALFORMED_ATTRIBUTE_LIST, nil, fmt.Sprintf("Invalid Endpoint Address length: %d", r.EndpointAddressLength))
	}
	r.EndpointAddress = ea
	r.TEID = binary.BigEndian.Uint32(data[p : p+teidLen])
	return nil
}

func (r *MUPType2SessionTransformedRoute) Serialize() ([]byte, error) {
	var buf []byte
	var err error
	if r.RD != nil {
		buf, err = r.RD.Serialize()
		if err != nil {
			return nil, err
		}
	} else {
		buf = make([]byte, 8)
	}
	buf = append(buf, r.EndpointAddressLength)
	buf = append(buf, r.EndpointAddress.AsSlice()...)
	t := make([]byte, 4)
	binary.BigEndian.PutUint32(t, r.TEID)
	buf = append(buf, t...)
	return buf, nil
}

func (r *MUPType2SessionTransformedRoute) AFI() uint16 {
	if r.EndpointAddress.Is6() {
		return AFI_IP6
	}
	return AFI_IP
}

func (r *MUPType2SessionTransformedRoute) Len() int {
	// RD(8) + EndpointAddressLength(1) + EndpointAddress(4 or 16)
	// + TEID(4)
	// Endpoint Address Length includes TEID Length
	return 9 + int(r.EndpointAddressLength/8)
}

func (r *MUPType2SessionTransformedRoute) String() string {
	return fmt.Sprintf("[type:t2st][rd:%s][endpoint:%s][teid:%d]", r.RD, r.EndpointAddress, r.TEID)
}

func (r *MUPType2SessionTransformedRoute) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		RD              RouteDistinguisherInterface `json:"rd"`
		EndpointAddress string                      `json:"endpoint_address"`
		TEID            uint32                      `json:"teid"`
	}{
		RD:              r.RD,
		EndpointAddress: r.EndpointAddress.String(),
		TEID:            r.TEID,
	})
}

func (r *MUPType2SessionTransformedRoute) rd() RouteDistinguisherInterface {
	return r.RD
}
