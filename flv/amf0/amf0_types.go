package amf0

import (
	"encoding/binary"
	"math"
)

type Element interface {
	Type() DataType

	Marshal(dst []byte) (int, error)
}

type Null struct {
}

func (a Null) Type() DataType {
	return DataTypeNull
}

func (a Null) Marshal(dst []byte) (int, error) {
	return 0, nil
}

type Number float64

func (a Number) Type() DataType {
	return DataTypeNumber
}

func (a Number) Marshal(dst []byte) (int, error) {
	binary.BigEndian.PutUint64(dst, math.Float64bits(float64(a)))
	return 8, nil
}

type Boolean bool

func (a Boolean) Type() DataType {
	return DataTypeBoolean
}

func (a Boolean) Marshal(dst []byte) (int, error) {
	if a {
		dst[0] = 1
	} else {
		dst[0] = 0
	}

	return 1, nil
}

type Undefined struct {
}

func (a Undefined) Type() DataType {
	return DataTypeUnDefined
}

func (a Undefined) Marshal(dst []byte) (int, error) {
	return 0, nil
}

type Reference uint16

func (a Reference) Type() DataType {
	return DataTypeReference
}

func (a Reference) Marshal(dst []byte) (int, error) {
	binary.BigEndian.PutUint16(dst, uint16(a))
	return 2, nil
}

type ECMAArray struct {
	*Object
}

func (a ECMAArray) Type() DataType {
	return DataTypeECMAArray
}

type StrictArray []Element

func (s StrictArray) Type() DataType {
	return DataTypeStrictArray
}

func (s StrictArray) Marshal(dst []byte) (int, error) {
	return MarshalElements(s, dst)
}

type Date struct {
	zone uint16
	date float64
}

func (a Date) Type() DataType {
	return DataTypeDate
}

func (a Date) Marshal(dst []byte) (int, error) {
	binary.BigEndian.PutUint16(dst, a.zone)
	binary.BigEndian.PutUint16(dst[2:], a.zone)
	return 10, nil
}

type LongString string

func (a LongString) Type() DataType {
	return DataTypeLongString
}

func (a LongString) Marshal(dst []byte) (int, error) {
	length := uint32(len(a))
	binary.BigEndian.PutUint32(dst, length)
	copy(dst[4:], a)
	return int(4 + length), nil
}

type XMLDocument struct {
	LongString
}

func (a XMLDocument) Type() DataType {
	return DataTypeXMLDocument
}

type TypedObject struct {
	ClassName string
	*Object
}

func (a TypedObject) Type() DataType {
	return DataTypeTypedObject
}

func (a TypedObject) Marshal(dst []byte) (int, error) {
	n, err := String(a.ClassName).Marshal(dst)
	if err != nil {
		return 0, err
	}

	n2, err := a.Object.Marshal(dst[n:])
	if err != nil {
		return 0, err
	}

	return n + n2, nil
}

type String string

func (a String) Type() DataType {
	return DataTypeString
}

func (a String) Marshal(dst []byte) (int, error) {
	binary.BigEndian.PutUint16(dst, uint16(len(a)))
	copy(dst[2:], a)
	return 2 + len(a), nil
}

func MarshalElement(element Element, dst []byte) (int, error) {
	dst[0] = byte(element.Type())
	n, err := element.Marshal(dst[1:])
	if err != nil {
		return 0, err
	}

	return 1 + n, nil
}

func MarshalElements(elements []Element, dst []byte) (int, error) {
	var length int
	for _, element := range elements {
		n, err := MarshalElement(element, dst[length:])
		if err != nil {
			return 0, err
		}

		length += n
	}

	return length, nil
}
