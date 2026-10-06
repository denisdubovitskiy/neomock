// Package shapes — фикстуры с экзотическими формами аргументов и результатов
// для сквозных тестов эмиттера и рантайма (план, этап 8: регрессия типов).
package shapes

import "context"

// IDs — именованный слайс: история клонируется через slices.Clone (~[]E).
type IDs []string

// UserID — именованный целочисленный тип.
type UserID int

// Filter — структурный аргумент: DeepEqual целиком, Diff — по полям.
type Filter struct {
	Name  string
	Limit int
}

// Fn — функциональный тип: neomock.Equal совпадает только пару nil-значений.
type Fn func(x int) error

// Rich — интерфейс со всеми экзотическими формами аргументов и результатов.
type Rich interface {
	// Query — именованные типы, struct, func, chan, pointer, []byte, три результата.
	Query(ctx context.Context, f Filter, ids IDs, fn Fn, ch chan int, p *Filter, b []byte) (map[string]int, []UserID, error)
	// Map — map-аргумент: nil ≠ пустому; история клонируется maps.Clone.
	Map(ctx context.Context, in map[string]int) map[string]int
	// Slice — слайс-аргумент: nil ≠ пустому.
	Slice(ctx context.Context, in []string) []string
	// OnlyErr — единственный результат error (soft-режим оборачивает его).
	OnlyErr(ctx context.Context) error
	// NoArgs — метод без аргументов: match всегда true.
	NoArgs() (int, error)
	// Variadic — вариадик именованного слайса-элемента.
	Variadic(prefix string, parts ...string) int
}
