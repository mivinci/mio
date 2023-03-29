package mio

import (
	"io"
	"testing"
)

type mockTransport struct{}

func (t mockTransport) split() (io.ReadWriteCloser, io.ReadWriteCloser) {
	pr1, pw1 := io.Pipe()
	pr2, pw2 := io.Pipe()
	return &mockRWC{pr: pr1, pw: pw2}, &mockRWC{pr: pr2, pw: pw1}
}

type mockRWC struct {
	pr *io.PipeReader
	pw *io.PipeWriter
}

func (r *mockRWC) Read(p []byte) (int, error) {
	return r.pr.Read(p)
}

func (r *mockRWC) Write(p []byte) (int, error) {
	return r.pw.Write(p)
}

func (r *mockRWC) Close() error {
	if err := r.pw.Close(); err != nil {
		return err
	}
	return r.pr.Close()
}

func TestSession(t *testing.T) {

}
