package sseparser_test

import (
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
	}

	for _, test := range tests {
		field, err := sseparser.ParseField(test.input)
		require.NoError(t, err)
		assert.Equal(t, test.expected, field)
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
	}

	for _, test := range tests {
		event, err := sseparser.ParseEvent(test.input)
		require.NoError(t, err)
		assert.Equal(t, test.fields, event.Fields())
		assert.Equal(t, test.comments, event.Comments())
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
	}

	for _, test := range tests {
		stream, err := sseparser.ParseStream(test.input)
		require.NoError(t, err)
		assert.Equal(t, test.expected, stream)
	}
}
