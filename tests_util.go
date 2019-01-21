package main

import (
	//	"fmt"
	"bytes"
	"go/ast"
	"go/importer"
	"go/parser"
	"go/token"
	"go/types"
	"log"
	"strings"
)

func extractor(xmlStrings []string) error {
	var attributePrefix = "Attr"
	var nameSpaceInJsonName = false

	var ignoreLowerCaseXmlTags = false
	var ignoredXmlTagsMap *map[string]struct{}

	var ignoreXmlDecodingErrors = false

	// Java out
	const javaBasePackage = "ca.gnewton.chidley"
	const mavenJavaBase = "src/main/java"

	var namePrefix = "C"
	var nameSuffix = ""

	var useType = false
	var flattenStrings = false

	//FIXXX: should not be global
	var keepXmlFirstLetterCase = true

	log.SetFlags(log.LstdFlags | log.Lshortfile)
	ex := Extractor{
		namePrefix:              namePrefix,
		nameSuffix:              nameSuffix,
		useType:                 useType,
		progress:                progress,
		ignoreXmlDecodingErrors: ignoreXmlDecodingErrors,
		ignoreLowerCaseXmlTags:  ignoreLowerCaseXmlTags,
		ignoredXmlTagsMap:       ignoredXmlTagsMap,
	}

	ex.init()

	for i, _ := range xmlStrings {
		//log.Println(xmlStrings[i])
		err := ex.extract(strings.NewReader(xmlStrings[i]))

		if err != nil {
			return err
		}
	}

	ex.done()

	buf := bytes.NewBufferString("")
	fps := make([]string, 1)
	//fps[0] = "foo"
	generateGoCode(buf, fps, &ex, flattenStrings, useType, keepXmlFirstLetterCase, nameSpaceInJsonName, namePrefix, nameSuffix, attributePrefix)

	//log.Println(buf.String())

	return parseAndType(buf.String())
}

// Derived from example at https://github.com/golang/example/tree/master/gotypes#an-example
func parseAndType(s string) error {
	fset := token.NewFileSet()
	//f, err := parser.ParseFile(fset, "hello.go", hello, 0)
	f, err := parser.ParseFile(fset, "hello.go", s, 0)
	if err != nil {
		//t.Error("Expected 1.5, got ", v)
		//log.Fatal(err) // parse error
		return err
	}

	conf := types.Config{Importer: importer.Default()}

	// Type-check the package containing only file f.
	// Check returns a *types.Package.
	_, err = conf.Check("cmd/hello", fset, []*ast.File{f}, nil)
	if err != nil {
		//log.Fatal(err) // type error
		return err
	}
	return nil
}
