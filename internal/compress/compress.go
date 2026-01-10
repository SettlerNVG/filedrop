package compress

import (
	"compress/gzip"
	"io"
)

// CompressedWriter wraps writer with gzip compression
type CompressedWriter struct {
	gw *gzip.Writer
}

// NewCompressedWriter creates gzip writer
func NewCompressedWriter(w io.Writer) *CompressedWriter {
	return &CompressedWriter{
		gw: gzip.NewWriter(w),
	}
}

// Write compresses and writes data
func (cw *CompressedWriter) Write(p []byte) (int, error) {
	return cw.gw.Write(p)
}

// Close flushes and closes gzip writer
func (cw *CompressedWriter) Close() error {
	return cw.gw.Close()
}

// Flush flushes buffered data
func (cw *CompressedWriter) Flush() error {
	return cw.gw.Flush()
}

// CompressedReader wraps reader with gzip decompression
type CompressedReader struct {
	gr *gzip.Reader
}

// NewCompressedReader creates gzip reader
func NewCompressedReader(r io.Reader) (*CompressedReader, error) {
	gr, err := gzip.NewReader(r)
	if err != nil {
		return nil, err
	}
	return &CompressedReader{gr: gr}, nil
}

// Read decompresses and reads data
func (cr *CompressedReader) Read(p []byte) (int, error) {
	return cr.gr.Read(p)
}

// Close closes gzip reader
func (cr *CompressedReader) Close() error {
	return cr.gr.Close()
}
