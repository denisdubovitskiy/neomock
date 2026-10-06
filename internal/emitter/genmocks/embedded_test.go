package genmocks

// Behavior-тесты встраивания интерфейсов (фикстура embedded, DESIGN §3.4):
// io-композиция, ромб с дедупликацией, gRPC-автовстраивание, инстанциация
// generic-интерфейса.

import (
	"bytes"
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	neomock "github.com/denisdubovitskiy/neomock"
	"github.com/denisdubovitskiy/neomock/internal/loader/testdata/embedded"
)

// TestReaderWriter_IoCopy — мок в реальной io-композиции: Read через Run
// (out-параметр — буфер заполняет мок), Write через Run со сбором данных.
func TestReaderWriter_IoCopy(t *testing.T) {
	t.Parallel()

	// arrange
	f := &fakeT{}
	m := NewMockReaderWriter(f)
	data := []byte("hello neomock")
	var readCalls int
	m.RunRead(func(p []byte) (int, error) {
		readCalls++
		if readCalls == 1 {
			return copy(p, data), nil
		}
		return 0, io.EOF
	})
	var written []byte
	m.RunWrite(func(p []byte) (int, error) {
		written = append(written, p...)
		return len(p), nil
	})

	// act
	var buf bytes.Buffer
	_, errRead := io.Copy(&buf, m)
	_, errWrite := io.Copy(m, strings.NewReader("out"))
	f.runCleanups()

	// assert
	require.NoError(t, errRead, "io.Copy(src=mock)")
	assert.Equal(t, "hello neomock", buf.String())
	require.NoError(t, errWrite, "io.Copy(dst=mock)")
	assert.Equal(t, "out", string(written))
	assert.Equal(t, 2, m.ReadCallsCount())
	assert.Equal(t, 1, m.WriteCallsCount())
	assert.Zero(
		t,
		f.errorCount(),
		"Run without When must be silent in verify: %s",
		f.allErrors(),
	)
}

// TestDiamond_SingleGet — ромб A{D}, B{D}, Diamond{A;B}: один метод в моке,
// реализация удовлетворяет и D, и Diamond.
func TestDiamond_SingleGet(t *testing.T) {
	t.Parallel()

	// arrange
	m := NewMockDiamond(&fakeT{})
	m.WhenGet().Then("x")
	var d embedded.D = m
	var diamond embedded.Diamond = m

	// act
	gotD := d.Get()
	gotDiamond := diamond.Get()

	// assert
	assert.Equal(t, "x", gotD, "D.Get")
	assert.Equal(t, "x", gotDiamond, "Diamond.Get")
}

// TestGreet_AutoEmbedSatisfiesInterface — gRPC-подобный интерфейс: mock
// удовлетворяет и экспортированным методам, и unexported-требованию
// mustEmbedGreet (за счёт автовстраивания, §3.4).
func TestGreet_AutoEmbedSatisfiesInterface(t *testing.T) {
	t.Parallel()

	ctx := t.Context()

	// arrange
	m := NewMockGreet(&fakeT{})
	var g embedded.Greet = m // conversion проходит: mustEmbedGreet удовлетворён встраиванием
	_ = g
	m.WhenHello(ctx, "den").Then("hi", nil)

	// act
	got, err := m.Hello(ctx, "den")

	// assert
	require.NoError(t, err)
	assert.Equal(t, "hi", got)
}

// TestIntStore_InstantiatedGeneric — встраивание pkg.Generic[int] развёрнуто:
// мок не параметризован, метод типизирован int.
func TestIntStore_InstantiatedGeneric(t *testing.T) {
	t.Parallel()

	ctx := t.Context()

	// arrange
	f := &fakeT{}
	m := NewMockIntStore(f) // без [T] — инстанциация уже подставлена
	m.WhenGet(ctx, 7).Then(8, nil)

	// act
	got, err := m.Get(ctx, 7)
	_, errUnexpected := m.Get(ctx, 9) // несовпавший вызов — soft-режим
	f.runCleanups()

	// assert
	require.NoError(t, err)
	assert.Equal(t, 8, got)
	assert.ErrorIs(t, errUnexpected, neomock.ErrUnexpectedCall)
	assert.Equal(
		t,
		1,
		f.errorCount(),
		"exactly one unexpected-call report: %s",
		f.allErrors(),
	)
}
