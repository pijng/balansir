package cacheutil

//Header ...
type Header struct {
	Key   string
	Value []string
}

//Body ...
type Body []byte

//Response ...
type Response struct {
	Headers []Header
	Body    Body
}
