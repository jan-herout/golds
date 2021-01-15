package server

import (
	//"bytes"
	"fmt"
	"go/build"
	"io"
	"net/http"
	"strings"
	"sync"

	"go101.org/golds/internal/server/translations"
)

type PageOutputOptions struct {
	GoldsVersion string

	PreferredLang string

	NoIdentifierUsesPages bool
	PlainSourceCodePages  bool
	EmphasizeWDPkgs       bool

	// ToDo:
	ListUnexportedRes   bool
	WorkingDirectory    string
	WdPkgsListingManner string
}

var (
	testingMode = false
	genDocsMode = false

	buildIdUsesPages       = true  // might be false in gen mode
	enableSoruceNavigation = true  // false to disable method implementation pages and some code reading features
	emphasizeWDPackages    = false // list packages in the current directory before other packages
	goldsVersion           string

	// ToDo: use this one to replace the above ones.
	pageOutputOptions PageOutputOptions
)

func setPageOutputOptions(options PageOutputOptions, forTesting bool) {
	buildIdUsesPages = !options.NoIdentifierUsesPages || forTesting
	enableSoruceNavigation = !options.PlainSourceCodePages || forTesting
	emphasizeWDPackages = options.EmphasizeWDPkgs || forTesting
	goldsVersion = options.GoldsVersion
}

type pageResType string

const (
	ResTypeNone           pageResType = ""
	ResTypeAPI            pageResType = "api"
	ResTypeModule         pageResType = "mod"
	ResTypePackage        pageResType = "pkg"
	ResTypeDependency     pageResType = "dep"
	ResTypeImplementation pageResType = "imp"
	ResTypeSource         pageResType = "src"
	ResTypeReference      pageResType = "use"
	ResTypeCSS            pageResType = "css"
	ResTypeJS             pageResType = "jvs"
	ResTypeSVG            pageResType = "svg"
	ResTypePNG            pageResType = "png"
)

func isHTMLPage(res pageResType) bool {
	switch res {
	default:
		panic("unknown resource type: " + res)
	case ResTypeAPI, ResTypeCSS, ResTypeJS, ResTypeSVG, ResTypePNG:
		return false
	case ResTypeNone:
	case ResTypeModule:
	case ResTypePackage:
	case ResTypeDependency:
	case ResTypeImplementation:
	case ResTypeSource:
	case ResTypeReference:
	}
	return true
}

type pageCacheKey struct {
	resType pageResType
	res     interface{}
	options interface{}
}

type pageCacheValue struct {
	data    []byte
	options interface{}
}

func (ds *docServer) cachePage(key pageCacheKey, data []byte) {
	if genDocsMode {
	} else if data == nil {
		delete(ds.cachedPages, key)
	} else {
		ds.cachedPages[key] = data
	}
}

func (ds *docServer) cachedPage(key pageCacheKey) (data []byte, ok bool) {
	if genDocsMode {
	} else {
		data, ok = ds.cachedPages[key]
	}
	return
}

func (ds *docServer) cachePageOptions(key pageCacheKey, options interface{}) {
	if genDocsMode {
	} else {
		key.options = nil
		ds.cachedPagesOptions[key] = options
	}
}

func (ds *docServer) cachedPageOptions(key pageCacheKey) (options interface{}) {
	if genDocsMode {
	} else {
		key.options = nil
		options = ds.cachedPagesOptions[key]
	}
	return
}

func addVersionToFilename(filename string, version string) string {
	return filename + "-" + version
}

func removeVersionFromFilename(filename string, version string) string {
	return strings.TrimSuffix(filename, "-"+version)
}

type htmlPage struct {
	//bytes.Buffer
	content Content

	goldsVersion string
	PathInfo     pagePathInfo

	// ToDo: use the two instead of server.currentXXXs.
	//theme Theme
	translation Translation

	isHTML bool
}

func (page *htmlPage) Translation() Translation {
	return page.translation
}

type pagePathInfo struct {
	resType pageResType
	resPath string
}

func NewHtmlPage(goldsVersion, title string, theme Theme, translation Translation, currentPageInfo pagePathInfo) *htmlPage {
	page := htmlPage{
		PathInfo:     currentPageInfo,
		goldsVersion: goldsVersion,
		translation:  translation,
		isHTML:       isHTMLPage(currentPageInfo.resType),
	}
	//page.Grow(4 * 1024 * 1024)

	if page.isHTML {
		fmt.Fprintf(&page, `<!DOCTYPE html>
<html>
<head>
<meta charset="utf-8">
<meta http-equiv="X-UA-Compatible" content="IE=edge">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>%s</title>
<link href="%s" rel="stylesheet">
<script src="%s"></script>
<body><div>
`,
			title,
			buildPageHref(currentPageInfo, pagePathInfo{ResTypeCSS, addVersionToFilename(theme.Name(), page.goldsVersion)}, nil, ""),
			buildPageHref(currentPageInfo, pagePathInfo{ResTypeJS, addVersionToFilename("golds", page.goldsVersion)}, nil, ""),
		)
	}

	return &page
}

// ToDo: w is not used now. It will be used if the page cache feature is remvoed later.s
func (page *htmlPage) Done(w io.Writer) []byte {
	if page.isHTML {
		var qrImgLink string
		switch page.translation.(type) {
		case *translations.Chinese:
			qrImgLink = buildPageHref(page.PathInfo, pagePathInfo{ResTypePNG, "go101-wechat"}, nil, "")
		case *translations.English:
			qrImgLink = buildPageHref(page.PathInfo, pagePathInfo{ResTypePNG, "go101-twitter"}, nil, "")
		}

		fmt.Fprintf(page, `<pre id="footer">
%s
</pre>`,
			page.translation.Text_GeneratedPageFooter(page.goldsVersion, qrImgLink, build.Default.GOOS, build.Default.GOARCH),
		)

		page.WriteString(`
</div></body></html>`,
		)
	}

	//return append([]byte(nil), page.Bytes()...)
	var data []byte
	if genDocsMode {
		w.(*docGenResponseWriter).content = page.content
		page.content = nil
		// The GenDocs function is in charge of collect page.content.
	} else {
		// w is a standard ResponseWriter.
		data = make([]byte, 0, page.content.DataLength())
		for _, bs := range page.content {
			//w.Write(bs)
			data = append(data, bs...)
		}
		contentPool.collect(page.content)
	}

	return data
}

func (page *htmlPage) writePageLink(writeHref func(), linkText string, fragments ...string) {
	if linkText != "" {
		page.WriteString(`<a href="`)
	}
	writeHref()
	if len(fragments) > 0 {
		page.WriteByte('#')
		for _, fm := range fragments {
			page.WriteString(fm)
		}
	}
	if linkText != "" {
		page.WriteString(`">`)
		page.WriteString(linkText)
		page.WriteString(`</a>`)
	}
}

func (page *htmlPage) Write(data []byte) (int, error) {
	dataLen := len(data)
	if dataLen != 0 {
		var bs []byte
		if page.content == nil {
			bs = contentPool.apply()[:0]
			page.content = [][]byte{bs, nil, nil}[:1]
		} else {
			bs = page.content[len(page.content)-1]
		}
		for len(data) > 0 {
			if len(bs) == cap(bs) {
				page.content[len(page.content)-1] = bs
				bs = contentPool.apply()[:0]
				page.content = append(page.content, bs)
			}

			n := copy(bs[len(bs):cap(bs)], data)
			bs = bs[:len(bs)+n]
			data = data[n:]
		}
		page.content[len(page.content)-1] = bs
	}

	return dataLen, nil
}

func (page *htmlPage) WriteString(data string) (int, error) {
	dataLen := len(data)
	if dataLen != 0 {
		var bs []byte
		if page.content == nil {
			bs = contentPool.apply()[:0]
			page.content = [][]byte{bs, nil, nil}[:1]
		} else {
			bs = page.content[len(page.content)-1]
		}
		for len(data) > 0 {
			if len(bs) == cap(bs) {
				page.content[len(page.content)-1] = bs
				bs = contentPool.apply()[:0]
				page.content = append(page.content, bs)
			}

			n := copy(bs[len(bs):cap(bs)], data)
			bs = bs[:len(bs)+n]
			data = data[n:]
		}
		page.content[len(page.content)-1] = bs
	}

	return dataLen, nil
}

func (page *htmlPage) WriteByte(c byte) error {
	var bs []byte
	if page.content == nil {
		bs = contentPool.apply()[:0]
		page.content = [][]byte{bs, nil, nil}[:1]
	} else {
		bs = page.content[len(page.content)-1]
	}
	if len(bs) == cap(bs) {
		bs = contentPool.apply()[:0]
		page.content = append(page.content, bs)
	}
	n := len(bs)
	bs = bs[:n+1]
	bs[n] = c
	page.content[len(page.content)-1] = bs
	return nil
}

//========================================
// Content is used to save memory allocations
//========================================

const Size = 1024 * 1024

type Content [][]byte // all []byte with capacity Size
type ContentPool struct {
	frees         Content
	mu            sync.Mutex
	numByteSlices int
}

var contentPool ContentPool

func (c Content) DataLength() int {
	if len(c) == 0 {
		return 0
	}
	return (len(c)-1)*Size + len(c[(len(c)-1)])
}

func (pool *ContentPool) apply() []byte {
	pool.mu.Lock()
	defer pool.mu.Unlock()
	if n := len(pool.frees); n == 0 {
		pool.numByteSlices++
		return make([]byte, Size)
	} else {
		n--
		bs := pool.frees[n]
		pool.frees = pool.frees[:n]
		return bs[:cap(bs)]
	}
}

func (pool *ContentPool) collect(c Content) {
	pool.mu.Lock()
	defer pool.mu.Unlock()
	if pool.frees == nil {
		pool.frees = make(Content, 0, 32)
	}
	pool.frees = append(pool.frees, c...)
}

//func readAll func(r io.Reader) (c Content, e error) {
//	for done := false; !done; {
//		bs, off := apply(), 0
//		for {
//			n, err := r.Read(bs[off:])
//			if err != nil {
//				if errors.Is(err, io.EOF) {
//					done = true
//				} else {
//					e = err
//					return
//				}
//			}
//			off += n
//			if done || off == cap(bs) {
//				c = append(c, bs[:off])
//				break
//			}
//		}
//	}
//	return
//}
//
//func writeFile func(path string, c Content) error {
//	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0644)
//	if err != nil {
//		return err
//	}
//	defer func() {
//		//release(c) // should not put here.
//		err = f.Close()
//	}()
//
//	for _, bs := range c {
//		_, err := f.Write(bs)
//		if err != nil {
//			return err
//		}
//	}
//
//	return nil
//}
//
//fakeServer := httptest.NewServer(http.HandlerFunc(ds.ServeHTTP))
//defer fakeServer.Close()
//
//buildPageContentFromFakeServer := func(path string) (Content, error) {
//	res, err := http.Get(fakeServer.URL + path)
//	if err != nil {
//		log.Fatal(err)
//	}
//
//	if res.StatusCode != http.StatusOK {
//		log.Fatalf("Visit %s, get non-ok status code: %d", path, res.StatusCode)
//	}
//
//	var content Content
//	if useCustomReadAllWriteFile {
//		content, err = readAll(res.Body)
//	} else {
//		var data []byte
//		data, err = ioutil.ReadAll(res.Body)
//		if err != nil {
//			content = append(content, data)
//		}
//	}
//	res.Body.Close()
//	return content, err
//}

type docGenResponseWriter struct {
	statusCode int
	header     http.Header
	content    Content
}

func (dw *docGenResponseWriter) reset() {
	dw.statusCode = http.StatusOK
	dw.content = nil
	for k := range dw.header {
		delete(dw.header, k)
	}
}

func (dw *docGenResponseWriter) Header() http.Header {
	if dw.header == nil {
		dw.header = make(http.Header, 3)
	}
	return dw.header
}

func (dw *docGenResponseWriter) WriteHeader(statusCode int) {
	dw.statusCode = statusCode
}

func (dw *docGenResponseWriter) Write(data []byte) (int, error) {
	return len(data), nil
}

//header := make(http.Header, 3)
//makeHeader := func() http.Header {
//	for k := range header {
//		header[k] = nil
//	}
//	return header
//}
//var responseWriter *docGenResponseWriter
//newResponseWriter := func(writeData func([]byte) (int, error)) *docGenResponseWriter {
//	if responseWriter == nil {
//		responseWriter = &docGenResponseWriter{}
//	}
//	responseWriter.statusCode = http.StatusOK
//	responseWriter.header = makeHeader()
//	responseWriter.writeData = writeData
//	return responseWriter
//}
//var fakeRequest *http.Request
//newRequest := func(path string) *http.Request {
//	if fakeRequest == nil {
//		req, err := http.NewRequest(http.MethodGet, "http://locahost", nil)
//		if err != nil {
//			log.Fatalln("Construct fake request error:", err)
//		}
//		fakeRequest = req
//	}
//	fakeRequest.URL.Path = path
//	return fakeRequest
//}
//buildPageContentUsingCustomWriter := func(path string) (Content, error) {
//	var content Content
//	var buf, off = apply(), 0
//	writeData := func(data []byte) (int, error) {
//		dataLen := len(data)
//		for len(data) > 0 {
//			if off == cap(buf) {
//				content = append(content, buf)
//				buf, off = apply(), 0
//			}
//			n := copy(buf[off:], data)
//			//log.Println("222", len(data), n, off, cap(buf))
//			data = data[n:]
//			off += n
//		}
//		return dataLen, nil
//	}
//
//	w := newResponseWriter(writeData)
//	r := newRequest(path)
//	ds.ServeHTTP(w, r)
//	buf = buf[:off]
//	content = append(content, buf)
//
//	if w.statusCode != http.StatusOK {
//		log.Fatalf("Build %s, get non-ok status code: %d", path, w.statusCode)
//	}
//
//	return content, nil
//}
