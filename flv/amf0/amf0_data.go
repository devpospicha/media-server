package amf0

import "github.com/devpospicha/media-server/avformat/bufio"

//@https://en.wikipedia.org/wiki/Action_Message_Format
//@https://rtmp.veriskope.com/pdf/amf0-file-format-specification.pdf

type DataType byte

const (
	DataTypeNumber       = DataType(0x00)
	DataTypeBoolean      = DataType(0x01)
	DataTypeString       = DataType(0x02)
	DataTypeObject       = DataType(0x03)
	DataTypeMovieClip    = DataType(0x04) // 预留字段
	DataTypeNull         = DataType(0x05)
	DataTypeUnDefined    = DataType(0x06)
	DataTypeReference    = DataType(0x07)
	DataTypeECMAArray    = DataType(0x08)
	DataTypeObjectEnd    = DataType(0x09)
	DataTypeStrictArray  = DataType(0x0A)
	DataTypeDate         = DataType(0x0B)
	DataTypeLongString   = DataType(0x0C)
	DataTypeUnsupported  = DataType(0x0D)
	DataTypeRecordSet    = DataType(0x0E) // 预留字段
	DataTypeXMLDocument  = DataType(0x0F)
	DataTypeTypedObject  = DataType(0x10)
	DataTypeSwitchTOAMF3 = DataType(0x11) // 切换到AMF3
)

type Data struct {
	elements []Element
}

func (a *Data) Marshal(dst []byte) (int, error) {
	return MarshalElements(a.elements, dst)
}

func (a *Data) Unmarshal(data []byte) error {
	buffer := bufio.NewBytesReader(data)

	for buffer.ReadableBytes() > 0 {
		element, err := ReadElement(buffer)
		if err != nil {
			return err
		}

		a.elements = append(a.elements, element)
	}

	return nil
}

func (a *Data) Size() int {
	return len(a.elements)
}

func (a *Data) Get(index int) Element {
	return a.elements[index]
}

func (a *Data) Add(value Element) {
	a.elements = append(a.elements, value)
}

func (a *Data) AddString(str string) {
	a.elements = append(a.elements, String(str))
}
func (a *Data) AddNumber(number float64) {
	a.elements = append(a.elements, Number(number))
}
