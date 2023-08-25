package sseparser_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"testing"

	"github.com/jclem/sseparser"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseField(t *testing.T) {
	tests := []struct {
		input    []byte
		expected sseparser.Field
	}{
		{
			[]byte("foo: bar\n"),
			sseparser.Field{"foo", "bar"},
		},
		{
			[]byte("foo:bar\n"),
			sseparser.Field{"foo", "bar"},
		},
		{
			[]byte("foo:\n"),
			sseparser.Field{"foo", ""},
		},
		{
			[]byte("foo:  \n"),
			sseparser.Field{"foo", " "},
		},
		{
			[]byte("foo bar\n"),
			sseparser.Field{"foo bar", ""},
		},
	}

	for _, test := range tests {
		field, err := sseparser.ParseField(test.input)
		require.NoError(t, err)
		assert.Equal(t, test.expected, field)
	}

	errtests := []struct {
		input []byte
		msg   string
	}{
		{
			[]byte("\n"),
			"unexpected input: \"\\n\"",
		},
		{
			[]byte("foo: bar"),
			"unexpected input: \"foo: bar\"",
		},
	}

	for _, test := range errtests {
		_, err := sseparser.ParseField(test.input)
		assert.EqualError(t, err, test.msg)
	}
}

func ExampleParseField() {
	field, err := sseparser.ParseField([]byte("foo: bar\n"))
	if err != nil {
		panic(err)
	}

	_, _ = fmt.Println(field.Name)
	_, _ = fmt.Println(field.Value)
	// Output:
	// foo
	// bar
}

func TestParseComment(t *testing.T) {
	tests := []struct {
		input    []byte
		expected sseparser.Comment
	}{
		{
			[]byte(":hello\n"),
			sseparser.Comment("hello"),
		},
		{
			[]byte(":\n"),
			sseparser.Comment(""),
		},
	}

	for _, test := range tests {
		comment, err := sseparser.ParseComment(test.input)
		require.NoError(t, err)
		assert.Equal(t, test.expected, comment)
	}

	errtests := []struct {
		input []byte
		msg   string
	}{
		{
			[]byte("hello\n"),
			"unexpected input: \"hello\\n\"",
		},
		{
			[]byte("hello"),
			"unexpected input: \"hello\"",
		},
	}

	for _, test := range errtests {
		_, err := sseparser.ParseComment(test.input)
		assert.EqualError(t, err, test.msg)
	}
}

func ExampleParseComment() {
	comment, err := sseparser.ParseComment([]byte(":hello\n"))
	if err != nil {
		panic(err)
	}

	_, _ = fmt.Println(comment)
	// Output:
	// hello
}

func TestParseEvent(t *testing.T) {
	tests := []struct {
		input    []byte
		fields   []sseparser.Field
		comments []sseparser.Comment
	}{
		{
			[]byte(":hello\n:bar\nfoo:bar\n\n"),
			[]sseparser.Field{
				{"foo", "bar"},
			},
			[]sseparser.Comment{
				sseparser.Comment("hello"),
				sseparser.Comment("bar"),
			},
		},
		{
			[]byte("\n"),
			[]sseparser.Field{},
			[]sseparser.Comment{},
		},
	}

	for _, test := range tests {
		event, err := sseparser.ParseEvent(test.input)
		require.NoError(t, err)
		assert.Equal(t, test.fields, event.Fields())
		assert.Equal(t, test.comments, event.Comments())
	}

	errtests := []struct {
		input []byte
		msg   string
	}{
		{
			[]byte("hello\n"),
			"unexpected input: \"hello\\n\"",
		},
		{
			[]byte("hello"),
			"unexpected input: \"hello\"",
		},
	}

	for _, test := range errtests {
		_, err := sseparser.ParseEvent(test.input)
		assert.EqualError(t, err, test.msg)
	}
}

func ExampleParseEvent() {
	event, err := sseparser.ParseEvent([]byte(":hello\n:bar\nfoo:bar\n\n"))
	if err != nil {
		panic(err)
	}

	_, _ = fmt.Printf("%+v\n", event)
	// Output:
	// [hello bar {Name:foo Value:bar}]
}

func TestParseStream(t *testing.T) {
	tests := []struct {
		input    []byte
		expected sseparser.Stream
	}{
		{
			[]byte(":hello\n:bar\nfoo:bar\n\nbaz:qux\n\n"),
			sseparser.Stream{
				{
					sseparser.Comment("hello"),
					sseparser.Comment("bar"),
					sseparser.Field{"foo", "bar"},
				},
				{
					sseparser.Field{"baz", "qux"},
				},
			},
		},
		{
			[]byte("\uFEFF:hello\n:bar\nfoo:bar\n\nbaz:qux\n\n"),
			sseparser.Stream{
				{
					sseparser.Comment("hello"),
					sseparser.Comment("bar"),
					sseparser.Field{"foo", "bar"},
				},
				{
					sseparser.Field{"baz", "qux"},
				},
			},
		},
	}

	for _, test := range tests {
		stream, err := sseparser.ParseStream(test.input)
		require.NoError(t, err)
		assert.Equal(t, test.expected, stream)
	}

	errtests := []struct {
		input []byte
		msg   string
	}{
		{
			[]byte("hello\n"),
			"unexpected input: \"hello\\n\"",
		},
		{
			[]byte("hello"),
			"unexpected input: \"hello\"",
		},
	}

	for _, test := range errtests {
		_, err := sseparser.ParseStream(test.input)
		assert.EqualError(t, err, test.msg)
	}
}

func ExampleParseStream() {
	stream, err := sseparser.ParseStream([]byte(":hello\n:bar\nfoo:bar\n\nbaz:qux\n\n"))
	if err != nil {
		panic(err)
	}

	_, _ = fmt.Printf("%+v\n", stream)
	// Output:
	// [[hello bar {Name:foo Value:bar}] [{Name:baz Value:qux}]]
}

func TestStreamScanner(t *testing.T) {
	input := []byte(":event-1\nfield-1: value-1\nfield-2: value-2\n\n:event-2\nfield-3: value-3\n\nLEFTOVER")
	reader := bytes.NewReader(input)

	scanner := sseparser.NewStreamScanner(reader)

	expected := []sseparser.Event{
		{
			sseparser.Comment("event-1"),
			sseparser.Field{"field-1", "value-1"},
			sseparser.Field{"field-2", "value-2"},
		},
		{
			sseparser.Comment("event-2"),
			sseparser.Field{"field-3", "value-3"},
		},
	}

	e, _, err := scanner.Next()
	require.NoError(t, err)
	assert.Equal(t, expected[0], e)

	e, _, err = scanner.Next()
	require.NoError(t, err)
	assert.Equal(t, expected[1], e)

	e, b, err := scanner.Next()
	assert.Nil(t, e)
	assert.ErrorIs(t, err, sseparser.ErrStreamEOF)
	assert.ErrorIs(t, err, io.EOF)
	assert.Equal(t, []byte("LEFTOVER"), b)
}

func ExampleStreamScanner() {
	input := []byte(`:event-1
field-1: value-1
field-2: value-2

:event-2
field-3: value-3

`)
	reader := bytes.NewReader(input)

	scanner := sseparser.NewStreamScanner(reader)

	for {
		e, _, err := scanner.Next()
		if err != nil {
			if errors.Is(err, sseparser.ErrStreamEOF) {
				break
			}

			panic(err)
		}

		_, _ = fmt.Printf("%+v\n", e)
	}

	// Output:
	// [event-1 {Name:field-1 Value:value-1} {Name:field-2 Value:value-2}]
	// [event-2 {Name:field-3 Value:value-3}]
}

func TestStreamScanner_UnmarshalNext(t *testing.T) {
	input := []byte(`:event-1
foo: 1
bar: hello
field-1: value-1
field-2: value-2

:event-2
foo: 1
field-3: value-3

foo: true

meta: {"foo": "bar"}

`)

	type testStruct struct {
		Foo string `sse:"foo"`
		Bar string `sse:"bar"`
	}

	type testStruct2 struct {
		Foo int `sse:"foo"`
	}

	type testStruct3 struct {
		Foo bool `sse:"foo"`
	}

	type testStruct4 struct {
		Meta meta `sse:"meta"`
	}

	reader := bytes.NewReader(input)
	scanner := sseparser.NewStreamScanner(reader)

	var event testStruct
	_, err := scanner.UnmarshalNext(&event)
	require.NoError(t, err)
	assert.Equal(t, testStruct{"1", "hello"}, event)

	var event2 testStruct2
	_, err = scanner.UnmarshalNext(&event2)
	require.NoError(t, err)
	assert.Equal(t, testStruct2{1}, event2)

	var event3 testStruct3
	_, err = scanner.UnmarshalNext(&event3)
	require.NoError(t, err)
	assert.Equal(t, testStruct3{true}, event3)

	var event4 testStruct4
	_, err = scanner.UnmarshalNext(&event4)
	require.NoError(t, err)
	assert.Equal(t, testStruct4{meta{"bar"}}, event4)
}

func ExampleStreamScanner_UnmarshalNext() {
	input := []byte(`:event-1
foo: 1
bar: hello
field-1: value-1
field-2: value-2

:event-2
foo: 1
field-3: value-3

foo: true

`)

	type testStruct struct {
		Foo string `sse:"foo"`
		Bar string `sse:"bar"`
	}

	reader := bytes.NewReader(input)
	scanner := sseparser.NewStreamScanner(reader)

	for {
		var event testStruct
		_, err := scanner.UnmarshalNext(&event)
		if err != nil {
			if errors.Is(err, sseparser.ErrStreamEOF) {
				break
			}

			panic(err)
		}

		_, _ = fmt.Printf("%+v\n", event)
	}

	// Output:
	// {Foo:1 Bar:hello}
	// {Foo:1 Bar:}
	// {Foo:true Bar:}
}

func ExampleUnmarshalerSSE() {
	input := []byte(`:event-1
foo: 1
bar: hello

`)

	type testStruct struct {
		Foo string `sse:"foo"`
		Bar string `sse:"bar"`
	}

	reader := bytes.NewReader(input)
	scanner := sseparser.NewStreamScanner(reader)

	var s testStruct
	_, err := scanner.UnmarshalNext(&s)
	if err != nil {
		panic(err)
	}

	_, _ = fmt.Printf("%+v\n", s)
	// Output:
	// {Foo:1 Bar:hello}
}

func ExampleUnmarshalerSSEValue() {
	input := []byte(`:event-1
meta: {"foo":"bar"}

`)

	// Meta implements the UnmarshalerSSEValue interface:
	//
	// type meta struct {
	// 	Foo string `json:"foo"`
	// }
	//
	// func (m *meta) UnmarshalSSEValue(v string) error {
	// 	return json.Unmarshal([]byte(v), m)
	// }

	type testStruct struct {
		Meta meta `sse:"meta"`
	}

	reader := bytes.NewReader(input)
	scanner := sseparser.NewStreamScanner(reader)

	var s testStruct
	_, err := scanner.UnmarshalNext(&s)
	if err != nil {
		panic(err)
	}

	_, _ = fmt.Printf("%+v\n", s)
	// Output:
	// {Meta:{Foo:bar}}
}

type meta struct {
	Foo string `json:"foo"`
}

func (m *meta) UnmarshalSSEValue(v string) error {
	if err := json.Unmarshal([]byte(v), m); err != nil {
		return fmt.Errorf("failed to unmarshal meta: %w", err)
	}

	return nil
}
