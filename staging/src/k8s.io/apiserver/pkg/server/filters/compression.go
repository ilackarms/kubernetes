package filters

import (
	"compress/gzip"
	"compress/zlib"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/emicklei/go-restful"
)

type Compressor interface {
	io.WriteCloser
	Flush() error
}

const (
	header_AcceptEncoding  = "Accept-Encoding"
	header_ContentEncoding = "Content-Encoding"

	encoding_gzip    = "gzip"
	encoding_deflate = "deflate"
)

func WithCompression(handler http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		//don't compress watches
		if req.URL.Query().Get("watch") == "true" {
			handler.ServeHTTP(w, req)
			return
		}
		wantsCompression, encoding := wantsCompressedResponse(req)
		if wantsCompression {
			compressionWriter := NewCompressionResponseWriter(w, encoding)
			compressionWriter.Header().Set("Content-Encoding", encoding)
			handler.ServeHTTP(compressionWriter, req)
		}
	})

}

// WantsCompressedResponse reads the Accept-Encoding header to see if and which encoding is requested.
func wantsCompressedResponse(httpRequest *http.Request) (bool, string) {
	header := httpRequest.Header.Get(header_AcceptEncoding)
	gi := strings.Index(header, encoding_gzip)
	zi := strings.Index(header, encoding_deflate)
	// use in order of appearance
	if gi == -1 {
		return zi != -1, encoding_deflate
	} else if zi == -1 {
		return gi != -1, encoding_gzip
	} else {
		if gi < zi {
			return true, encoding_gzip
		}
		return true, encoding_deflate
	}
}

type compressionResponseWriter struct {
	writer     http.ResponseWriter
	compressor Compressor
	encoding   string
}

func NewCompressionResponseWriter(w http.ResponseWriter, encoding string) *compressionResponseWriter {
	var compressor Compressor
	switch encoding {
	case encoding_gzip:
		compressor = gzip.NewWriter(w)
	case encoding_deflate:
		compressor = zlib.NewWriter(w)
	default:
		panic(encoding + " not a valid compression type")
	}
	return &compressionResponseWriter{
		writer:     w,
		compressor: compressor,
		encoding:   encoding,
	}
}

// compressionResponseWriter implements http.Responsewriter Interface
var _ http.ResponseWriter = &compressionResponseWriter{}

func (c *compressionResponseWriter) Header() http.Header {
	return c.writer.Header()
}

// compress data according to compression method
func (c *compressionResponseWriter) Write(p []byte) (int, error) {
	if c.isCompressorClosed() {
		return -1, errors.New("Compressing error: tried to write data using closed compressor")
	}
	c.Header().Set(header_ContentEncoding, c.encoding)
	//glog.V(0).Infof("WROTE \n\n%s\n\nwith headers: %+v\n", p, c.Header())
	defer c.Close()
	return c.compressor.Write(p)
}

func (c *compressionResponseWriter) WriteHeader(status int) {
	c.writer.WriteHeader(status)
}

// CloseNotify is part of http.CloseNotifier interface
func (c *compressionResponseWriter) CloseNotify() <-chan bool {
	return c.writer.(http.CloseNotifier).CloseNotify()
}

// Close the underlying compressor
func (c *compressionResponseWriter) Close() error {
	if c.isCompressorClosed() {
		return errors.New("Compressing error: tried to close already closed compressor")
	}

	c.compressor.Close()
	c.compressor = nil
	return nil
}

func (c *compressionResponseWriter) Flush() {
	c.compressor.Flush()
}

func (c *compressionResponseWriter) isCompressorClosed() bool {
	return nil == c.compressor
}

func RestfulWithCompression(function restful.RouteFunction) restful.RouteFunction {
	return restful.RouteFunction(func(request *restful.Request, response *restful.Response) {
		//don't compress watches
		wantsCompression, encoding := wantsCompressedResponse(request.Request)
		if wantsCompression && request.QueryParameter("watch") != "true" && request.QueryParameter("watch") != "1" {
			compressionWriter := NewCompressionResponseWriter(response.ResponseWriter, encoding)
			compressionWriter.Header().Set("Content-Encoding", encoding)
			response.ResponseWriter = compressionWriter
		}
		function(request, response)
	})
}
