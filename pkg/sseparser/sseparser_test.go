package sseparser_test

import (
	"bytes"
	"io"
	"testing"

	"github.com/jclem/sseparser/pkg/sseparser"
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
			"Unexpected input: \"\\n\"",
		},
		{
			[]byte("foo: bar"),
			"Unexpected input: \"foo: bar\"",
		},
	}

	for _, test := range errtests {
		_, err := sseparser.ParseField(test.input)
		assert.EqualError(t, err, test.msg)
	}
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
			"Unexpected input: \"hello\\n\"",
		},
		{
			[]byte("hello"),
			"Unexpected input: \"hello\"",
		},
	}

	for _, test := range errtests {
		_, err := sseparser.ParseComment(test.input)
		assert.EqualError(t, err, test.msg)
	}
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
			"Unexpected input: \"hello\\n\"",
		},
		{
			[]byte("hello"),
			"Unexpected input: \"hello\"",
		},
	}

	for _, test := range errtests {
		_, err := sseparser.ParseEvent(test.input)
		assert.EqualError(t, err, test.msg)
	}
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
			"Unexpected input: \"hello\\n\"",
		},
		{
			[]byte("hello"),
			"Unexpected input: \"hello\"",
		},
	}

	for _, test := range errtests {
		_, err := sseparser.ParseStream(test.input)
		assert.EqualError(t, err, test.msg)
	}
}

func TestStreamScanner(t *testing.T) {
	input := []byte(":event-1\nfield-1: value-1\nfield-2: value-2\n\n:event-2\nfield-3: value-3\n\n")
	reader := bytes.NewReader(input)

	scanner := sseparser.NewStreamScanner(reader)

	expected := []*sseparser.Event{
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

	e, err := scanner.Next()
	require.NoError(t, err)
	assert.Equal(t, expected[0], e)

	e, err = scanner.Next()
	require.NoError(t, err)
	assert.Equal(t, expected[1], e)

	e, err = scanner.Next()
	assert.Nil(t, e)
	assert.ErrorIs(t, err, io.EOF)
}
