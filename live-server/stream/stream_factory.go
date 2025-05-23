package stream

import (
	"fmt"
)

type TransStreamFactory func(source Source, protocol TransStreamProtocol, tracks []*Track) (TransStream, error)

type RecordStreamFactory func(source string) (Sink, string, error)

var (
	transStreamFactories map[TransStreamProtocol]TransStreamFactory
	recordStreamFactory  RecordStreamFactory
)

func init() {
	transStreamFactories = make(map[TransStreamProtocol]TransStreamFactory, 8)
}

func RegisterTransStreamFactory(protocol TransStreamProtocol, streamFunc TransStreamFactory) {
	_, ok := transStreamFactories[protocol]
	if ok {
		panic(fmt.Sprintf("%s has been registered", protocol.String()))
	}

	transStreamFactories[protocol] = streamFunc
}

func FindTransStreamFactory(protocol TransStreamProtocol) (TransStreamFactory, error) {
	f, ok := transStreamFactories[protocol]
	if !ok {
		return nil, fmt.Errorf("unknown protocol %s", protocol.String())
	}

	return f, nil
}

func CreateTransStream(source Source, protocol TransStreamProtocol, tracks []*Track) (TransStream, error) {
	factory, err := FindTransStreamFactory(protocol)
	if err != nil {
		return nil, err
	}

	return factory(source, protocol, tracks)
}

func SetRecordStreamFactory(factory RecordStreamFactory) {
	recordStreamFactory = factory
}

func CreateRecordStream(sourceId string) (Sink, string, error) {
	return recordStreamFactory(sourceId)
}
