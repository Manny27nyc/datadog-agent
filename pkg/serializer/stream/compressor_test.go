// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018-present Datadog, Inc.

//+build zlib

package stream

import (
	"bytes"
	"compress/zlib"
	"errors"
	"fmt"
	"io/ioutil"
	"testing"

	jsoniter "github.com/json-iterator/go"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/serializer/marshaler"
)

var (
	maxPayloadSizeDefault = config.Datadog.GetInt("serializer_max_payload_size")
)

type dummyMarshaller struct {
	items  []string
	header string
	footer string
}

func resetDefaults() {
	config.Datadog.SetDefault("serializer_max_payload_size", maxPayloadSizeDefault)
}

func (d *dummyMarshaller) WriteHeader(stream *jsoniter.Stream) error {
	_, err := stream.Write([]byte(d.header))
	return err
}

func (d *dummyMarshaller) Len() int {
	return len(d.items)
}

func (d *dummyMarshaller) WriteItem(stream *jsoniter.Stream, i int) error {
	if i < 0 || i > d.Len()-1 {
		return errors.New("out of range")
	}
	_, err := stream.Write([]byte(d.items[i]))
	return err
}

func (d *dummyMarshaller) DescribeItem(i int) string {
	if i < 0 || i > d.Len()-1 {
		return "out of range"
	}
	return d.items[i]
}

func (d *dummyMarshaller) WriteFooter(stream *jsoniter.Stream) error {
	_, err := stream.Write([]byte(d.footer))
	return err
}

func (d *dummyMarshaller) MarshalJSON() ([]byte, error) {
	return nil, fmt.Errorf("not implemented")
}

func (d *dummyMarshaller) Marshal() ([]byte, error) {
	return nil, fmt.Errorf("not implemented")
}

func (d *dummyMarshaller) SplitPayload(int) ([]marshaler.AbstractMarshaler, error) {
	return nil, fmt.Errorf("not implemented")
}

func (d *dummyMarshaller) MarshalSplitCompress(bufferContext *marshaler.BufferContext) ([]*[]byte, error) {
	return nil, fmt.Errorf("not implemented")
}

func decompressPayload(payload []byte) ([]byte, error) {
	r, err := zlib.NewReader(bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	defer r.Close()

	dst, err := ioutil.ReadAll(r)
	if err != nil {
		return nil, err
	}
	return dst, nil
}

func payloadToString(payload []byte) string {
	p, err := decompressPayload(payload)
	if err != nil {
		return err.Error()
	}
	return string(p)
}

func TestCompressorSimple(t *testing.T) {
	c, err := NewCompressor(&bytes.Buffer{}, &bytes.Buffer{}, []byte("{["), []byte("]}"), []byte(","))
	require.NoError(t, err)

	for i := 0; i < 5; i++ {
		c.AddItem([]byte("A"))
	}

	p, err := c.Close()
	require.NoError(t, err)
	require.Equal(t, "{[A,A,A,A,A]}", payloadToString(p))
}

func TestOnePayloadSimple(t *testing.T) {
	m := &dummyMarshaller{
		items:  []string{"A", "B", "C"},
		header: "{[",
		footer: "]}",
	}

	builder := NewJSONPayloadBuilder(true)
	payloads, err := builder.Build(m)
	require.NoError(t, err)
	require.Len(t, payloads, 1)

	require.Equal(t, "{[A,B,C]}", payloadToString(*payloads[0]))
}

func TestMaxCompressedSizePayload(t *testing.T) {
	m := &dummyMarshaller{
		items:  []string{"A", "B", "C"},
		header: "{[",
		footer: "]}",
	}
	config.Datadog.SetDefault("serializer_max_payload_size", 22)
	defer resetDefaults()

	builder := NewJSONPayloadBuilder(true)
	payloads, err := builder.Build(m)
	require.NoError(t, err)
	require.Len(t, payloads, 1)

	require.Equal(t, "{[A,B,C]}", payloadToString(*payloads[0]))
}

func TestTwoPayload(t *testing.T) {
	m := &dummyMarshaller{
		items:  []string{"A", "B", "C", "D", "E", "F"},
		header: "{[",
		footer: "]}",
	}
	config.Datadog.SetDefault("serializer_max_payload_size", 22)
	defer resetDefaults()

	builder := NewJSONPayloadBuilder(true)
	payloads, err := builder.Build(m)
	require.NoError(t, err)
	require.Len(t, payloads, 2)

	require.Equal(t, "{[A,B,C]}", payloadToString(*payloads[0]))
	require.Equal(t, "{[D,E,F]}", payloadToString(*payloads[1]))
}

func TestLockedCompressorProducesSamePayloads(t *testing.T) {
	m := &dummyMarshaller{
		items:  []string{"A", "B", "C", "D", "E", "F"},
		header: "{[",
		footer: "]}",
	}
	defer resetDefaults()

	builderLocked := NewJSONPayloadBuilder(true)
	builderUnLocked := NewJSONPayloadBuilder(false)
	payloads1, err := builderLocked.Build(m)
	require.NoError(t, err)
	payloads2, err := builderUnLocked.Build(m)
	require.NoError(t, err)

	require.Equal(t, payloadToString(*payloads1[0]), payloadToString(*payloads2[0]))
}
