package amf0

import (
	"math"

	"github.com/devpospicha/media-server/avformat/bufio"
)

func ReadString(buffer bufio.BytesReader) (string, error) {
	size, err := buffer.ReadUint16()
	if err != nil {
		return "", err
	}

	bytes, err := buffer.ReadBytes(int(size))
	if err != nil {
		return "", err
	}

	return string(bytes), nil
}

func ReadLongString(buffer bufio.BytesReader) (string, error) {
	size, err := buffer.ReadUint32()
	if err != nil {
		return "", err
	}

	bytes, err := buffer.ReadBytes(int(size))
	if err != nil {
		return "", err
	}

	return string(bytes), nil
}

func ReadObjectProperties(buffer bufio.BytesReader) (*Object, error) {
	object := &Object{}
	for buffer.ReadableBytes() >= 3 {
		endMark, _ := buffer.ReadUint24()
		if uint32(DataTypeObjectEnd) == endMark {
			return object, nil
		}

		_ = buffer.SeekBack(3)
		key, err := ReadString(buffer)
		if err != nil {
			return nil, err
		}

		value, err := ReadElement(buffer)
		if err != nil {
			return nil, err
		}

		object.AddProperty(key, value)
	}

	return object, nil
}

func ReadElement(buffer bufio.BytesReader) (Element, error) {
	marker, err := buffer.ReadUint8()
	if err != nil {
		return nil, err
	}

	switch DataType(marker) {
	case DataTypeNumber:
		number, err := buffer.ReadUint64()
		if err != nil {
			return nil, err
		}

		return Number(math.Float64frombits(number)), nil
	case DataTypeBoolean:
		value, err := buffer.ReadUint8()
		if err != nil {
			return nil, err
		}

		if 0 == value {
			return Boolean(false), nil
		} else {
			return Boolean(true), nil
		}
	case DataTypeString:
		amf0String, err := ReadString(buffer)
		if err != nil {
			return nil, err
		}

		return String(amf0String), nil
	case DataTypeObject:
		return ReadObjectProperties(buffer)
	case DataTypeMovieClip:
		println("skip reserved field MovieClip")
		return nil, nil
	case DataTypeNull:
		return Null{}, nil
	case DataTypeUnDefined:
		return Undefined{}, nil
	case DataTypeReference:
		// 引用元素索引
		index, err := buffer.ReadUint32()
		if err != nil {
			return nil, err
		}

		return Reference(index), nil
	case DataTypeECMAArray:
		// count *(object-property)
		_, err := buffer.ReadUint32()
		if err != nil {
			return nil, err
		}
		//for i := 0; i < count; i++ {
		//
		//}

		object, err := ReadObjectProperties(buffer)
		if err != nil {
			return nil, err
		}

		return &ECMAArray{object}, nil
	case DataTypeStrictArray:
		// array-count *(value-type)
		var array []Element
		count, err := buffer.ReadUint32()
		if err != nil {
			return nil, err
		}

		for i := 0; i < int(count); i++ {
			element, err := ReadElement(buffer)
			if err != nil {
				return nil, err
			}

			array = append(array, element)
		}

		return StrictArray(array), nil
	case DataTypeDate:

		zone, err := buffer.ReadUint16()
		if err != nil {
			return nil, err
		}

		date, err := buffer.ReadUint64()
		if err != nil {
			return nil, err
		}

		return Date{zone, math.Float64frombits(date)}, nil
	case DataTypeLongString:
		longString, err := ReadLongString(buffer)
		if err != nil {
			return nil, err
		}

		return LongString(longString), nil
	case DataTypeUnsupported, DataTypeRecordSet:
		return nil, nil
	case DataTypeXMLDocument:
		// The XML document type is always encoded as a long UTF-8 string.
		longString, err := ReadLongString(buffer)
		if err != nil {
			return nil, err
		}

		return XMLDocument{LongString(longString)}, nil
	case DataTypeTypedObject:
		className, err := ReadString(buffer)
		if err != nil {
			return nil, err
		}

		properties, err := ReadObjectProperties(buffer)
		return TypedObject{className, properties}, nil
	case DataTypeSwitchTOAMF3:
		return nil, nil
	}

	return nil, nil
}
