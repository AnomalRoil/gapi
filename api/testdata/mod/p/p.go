package p

import "io"

const Answer = 42

const Pi float64 = 3.14

var Default *Client

type Client struct {
	Name string
	io.Reader
	secret int
}

func NewClient(name string, opts ...Option) (*Client, error) { return nil, nil }

func (c *Client) Close() error { return nil }

func (c Client) String() string { return c.Name }

type Option func(*Client)

type Sealed interface {
	Do(int) bool
	sealed()
}

type Open interface {
	Get() string
	Set(string)
}

type Number interface {
	~int | ~float64
}

func Sum[T Number](xs []T) T { var t T; return t }

type Set[K comparable] map[K]struct{}

func (s Set[K]) Has(k K) bool { _, ok := s[k]; return ok }

type Alias = Client

func Events() <-chan Option { return nil }

type Key interface {
	comparable
	String() string
}

type Ints interface {
	~uint | ~int
	int
}

func Keys[K Key, V any](m map[K]V) []K { return nil }
