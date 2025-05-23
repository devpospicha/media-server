package amf0

import (
	"encoding/binary"

	"github.com/devpospicha/media-server/avformat/bufio"
)

type Object struct {
	properties []*Property
}

func (a *Object) Type() DataType {
	return DataTypeObject
}

func (a *Object) Marshal(dst []byte) (int, error) {
	var length int
	for _, property := range a.properties {
		n, err := property.Marshal(dst[length:])
		if err != nil {
			return 0, err
		}

		length += n
	}

	bufio.PutUint24(dst[length:], uint32(DataTypeObjectEnd))
	return length + 3, nil
}

func (a *Object) AddProperty(name string, value Element) {
	a.properties = append(a.properties, &Property{name, value})
}

func (a *Object) FindProperty(name string) *Property {
	for _, property := range a.properties {
		if property.Name == name {
			return property
		}
	}

	return nil
}

func (a *Object) AddStringProperty(name, value string) {
	a.AddProperty(name, String(value))
}

func (a *Object) AddNumberProperty(name string, value float64) {
	a.AddProperty(name, Number(value))
}

type Property struct {
	Name  string
	Value Element
}

func (a *Property) Marshal(dst []byte) (int, error) {
	length := len(a.Name)
	binary.BigEndian.PutUint16(dst, uint16(length))
	copy(dst[2:], a.Name)
	length += 2

	n, err := MarshalElement(a.Value, dst[length:])
	if err != nil {
		return 0, err
	}

	return length + n, nil
}
