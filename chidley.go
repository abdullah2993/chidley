package main

// Copyright 2014,2015,2016 Glen Newton
// glen.newton@gmail.com

import (
	"bufio"
	"bytes"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"text/template"
	"time"
)

func main() {

	var attributePrefix = "Attr"
	var codeGenConvert = false
	var nameSpaceInJsonName = false
	var readFromStandardIn = false
	var sortByXmlOrder = false
	var structsToStdout = true
	var validateFieldTemplate = false

	var ignoreLowerCaseXmlTags = false
	var ignoredXmlTags = ""
	var ignoredXmlTagsMap *map[string]struct{}

	var ignoreXmlDecodingErrors = false
	// Java out
	const javaBasePackage = "ca.gnewton.chidley"
	const mavenJavaBase = "src/main/java"

	var javaBasePackagePath = strings.Replace(javaBasePackage, ".", "/", -1)
	var javaAppName = "jaxb"
	var writeJava = false
	var baseJavaDir = "java"
	var userJavaPackageName = ""

	var namePrefix = "C"
	var nameSuffix = ""
	var url = false
	var useType = false
	var flattenStrings = false

	//FIXXX: should not be global
	var keepXmlFirstLetterCase = true

	var lengthTagName = ""
	var lengthTagAttribute = ""

	var outputs = []*bool{
		&codeGenConvert,
		&structsToStdout,
		&writeJava,
	}

	handleParameters := func() error {

		flag.BoolVar(&DEBUG, "d", DEBUG, "Debug; prints out much information")
		flag.BoolVar(&codeGenConvert, "W", codeGenConvert, "Generate Go code to convert XML to JSON or XML (latter useful for validation) and write it to stdout")
		flag.BoolVar(&flattenStrings, "F", flattenStrings, "Assume complete representative XML and collapse tags with only a single string and no attributes")
		flag.BoolVar(&ignoreXmlDecodingErrors, "I", ignoreXmlDecodingErrors, "If XML decoding error encountered, continue")
		flag.BoolVar(&nameSpaceInJsonName, "n", nameSpaceInJsonName, "Use the XML namespace prefix as prefix to JSON name")
		flag.BoolVar(&progress, "r", progress, "Progress: every 50000 input tags (elements)")
		flag.BoolVar(&readFromStandardIn, "c", readFromStandardIn, "Read XML from standard input")
		flag.BoolVar(&sortByXmlOrder, "X", sortByXmlOrder, "Sort output of structs in Go code by order encounered in source XML (default is alphabetical order)")
		flag.BoolVar(&structsToStdout, "G", structsToStdout, "Only write generated Go structs to stdout")
		flag.BoolVar(&url, "u", url, "Filename interpreted as an URL")
		flag.BoolVar(&useType, "t", useType, "Use type info obtained from XML (int, bool, etc); default is to assume everything is a string; better chance at working if XMl sample is not complete")
		flag.BoolVar(&writeJava, "J", writeJava, "Generated Java code for Java/JAXB")
		flag.BoolVar(&keepXmlFirstLetterCase, "K", keepXmlFirstLetterCase, "Do not change the case of the first letter of the XML tag names")
		flag.BoolVar(&validateFieldTemplate, "m", validateFieldTemplate, "Validate the field template. Useful to make sure the template defined with -T is valid")

		flag.BoolVar(&ignoreLowerCaseXmlTags, "L", ignoreLowerCaseXmlTags, "Ignore lower case XML tags")

		flag.StringVar(&attributePrefix, "a", attributePrefix, "Prefix to attribute names")
		flag.StringVar(&baseJavaDir, "D", baseJavaDir, "Base directory for generated Java code (root of maven project)")
		flag.StringVar(&fieldTemplateString, "T", fieldTemplateString, "Field template for the struct field definition. Can include annotations. Default is for XML and JSON")
		flag.StringVar(&javaAppName, "k", javaAppName, "App name for Java code (appended to ca.gnewton.chidley Java package name))")
		flag.StringVar(&lengthTagAttribute, "A", lengthTagAttribute, "The tag name attribute to use for the max length Go annotations")
		flag.StringVar(&lengthTagName, "N", lengthTagName, "The tag name to use for the max length Go annotations")
		flag.StringVar(&namePrefix, "e", namePrefix, "Prefix to struct (element) names; must start with a capital")
		flag.StringVar(&userJavaPackageName, "P", userJavaPackageName, "Java package name (rightmost in full package name")

		flag.StringVar(&ignoredXmlTags, "h", ignoredXmlTags, "List of XML tags to ignore; comma separated")

		flag.Parse()

		if codeGenConvert || writeJava {
			structsToStdout = false
		}

		numBoolsSet := countNumberOfBoolsSet(outputs)
		if numBoolsSet > 1 {
			log.Print("  ERROR: Only one of -W -J -X -V -c can be set")
		} else if numBoolsSet == 0 {
			log.Print("  ERROR: At least one of -W -J -X -V -c must be set")
		}
		if sortByXmlOrder {
			structSort = printStructsByXml
		}

		var err error
		ignoredXmlTagsMap, err = extractExcludedTags(ignoredXmlTags)
		if err != nil {
			return err
		}

		if lengthTagName == "" && lengthTagAttribute == "" || lengthTagName != "" && lengthTagAttribute != "" {
			return nil
		}

		return errors.New("Both lengthTagName and lengthTagAttribute must be set")
	}
	//log.Println(fieldTemplateString)

	//EXP()
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	err := handleParameters()

	if err != nil {
		log.Println(err)
		flag.Usage()
		return
	}

	err = runValidateFieldTemplate(validateFieldTemplate)
	if err != nil {

		return
	}
	if validateFieldTemplate {
		return
	}

	if len(flag.Args()) == 0 && !readFromStandardIn {
		fmt.Println("chidley <flags> xmlFileName|url")
		fmt.Println("xmlFileName can be .gz or .bz2: uncompressed transparently")
		flag.Usage()
		return
	}

	var sourceNames []string

	if !readFromStandardIn {
		sourceNames = flag.Args()
	}
	if !url && !readFromStandardIn {
		for i, _ := range sourceNames {
			sourceNames[i], err = filepath.Abs(sourceNames[i])
			if err != nil {
				log.Fatal("FATAL ERROR: " + err.Error())
			}
		}
	}

	sources, err := makeSourceReaders(sourceNames, url, readFromStandardIn)
	if err != nil {
		log.Fatal("FATAL ERROR: " + err.Error())
	}

	ex := Extractor{
		namePrefix:              namePrefix,
		nameSuffix:              nameSuffix,
		useType:                 useType,
		progress:                progress,
		ignoreXmlDecodingErrors: ignoreXmlDecodingErrors,
		initted:                 false,
		ignoreLowerCaseXmlTags:  ignoreLowerCaseXmlTags,
		ignoredXmlTagsMap:       ignoredXmlTagsMap,
	}

	if DEBUG {
		log.Print("extracting")
	}

	m := &ex
	m.init()

	for source := range sources {
		if DEBUG {
			log.Println("READER", source)
		}
		err = m.extract(source.getReader())

		if err != nil {
			log.Println("ERROR: " + err.Error())
			if !ignoreXmlDecodingErrors {
				log.Fatal("FATAL ERROR: " + err.Error())
			}
		}
		if DEBUG {
			log.Println("DONE READER", source)
		}
	}

	ex.done()

	switch {
	case codeGenConvert:
		generateGoCode(os.Stdout, sourceNames, &ex, flattenStrings, useType, keepXmlFirstLetterCase, nameSpaceInJsonName, namePrefix, nameSuffix, attributePrefix)

	case structsToStdout:
		generateGoStructs(os.Stdout, sourceNames[0], &ex, flattenStrings, useType, keepXmlFirstLetterCase, nameSpaceInJsonName, namePrefix, nameSuffix, attributePrefix)

	case writeJava:
		if len(userJavaPackageName) > 0 {
			javaAppName = userJavaPackageName
		}
		javaPackage := javaBasePackage + "." + javaAppName
		javaDir := baseJavaDir + "/" + mavenJavaBase + "/" + javaBasePackagePath + "/" + javaAppName

		os.RemoveAll(baseJavaDir)
		os.MkdirAll(javaDir+"/xml", 0755)
		date := time.Now()
		printJavaJaxbVisitor := PrintJavaJaxbVisitor{
			alreadyVisited:         make(map[string]bool),
			globalTagAttributes:    ex.globalTagAttributes,
			nameSpaceTagMap:        ex.nameSpaceTagMap,
			useType:                useType,
			javaDir:                javaDir,
			javaPackage:            javaPackage,
			namePrefix:             namePrefix,
			keepXmlFirstLetterCase: keepXmlFirstLetterCase,
			Date:                   date,
		}

		var onlyChild *Node
		for _, child := range ex.root.children {
			printJavaJaxbVisitor.Visit(child)
			// Bad: assume only one base element
			onlyChild = child
		}
		fullPath, err := getFullPath(sourceNames[0])
		if err != nil {
			log.Fatal(err)
		}
		printJavaJaxbMain(onlyChild.makeJavaType(namePrefix, "", keepXmlFirstLetterCase), javaDir, javaPackage, fullPath, date)
		printPackageInfo(onlyChild, javaDir, javaPackage, ex.globalTagAttributes, ex.nameSpaceTagMap)

		printMavenPom(baseJavaDir+"/pom.xml", javaAppName)
	}

}

func printPackageInfo(node *Node, javaDir string, javaPackage string, globalTagAttributes map[string][]*FQN, nameSpaceTagMap map[string]string) {

	//log.Printf("%+v\n", node)

	if node.space != "" {
		_ = findNameSpaces(globalTagAttributes[nk(node)])
		//attributes := findNameSpaces(globalTagAttributes[nk(node)])

		t := template.Must(template.New("package-info").Parse(jaxbPackageInfoTemplage))
		packageInfoPath := javaDir + "/xml/package-info.java"
		fi, err := os.Create(packageInfoPath)
		if err != nil {
			log.Print("Problem creating file: " + packageInfoPath)
			panic(err)
		}
		defer fi.Close()

		writer := bufio.NewWriter(fi)
		packageInfo := JaxbPackageInfo{
			BaseNameSpace: node.space,
			//AdditionalNameSpace []*FQN
			PackageName: javaPackage + ".xml",
		}
		err = t.Execute(writer, packageInfo)
		if err != nil {
			log.Println("executing template:", err)
		}
		bufio.NewWriter(writer).Flush()
	}

}

const XMLNS = "xmlns"

func findNameSpaces(attributes []*FQN) []*FQN {
	if attributes == nil || len(attributes) == 0 {
		return nil
	}
	xmlns := make([]*FQN, 0)
	return xmlns
}

func printMavenPom(pomPath string, javaAppName string) {
	t := template.Must(template.New("mavenPom").Parse(mavenPomTemplate))
	fi, err := os.Create(pomPath)
	if err != nil {
		log.Print("Problem creating file: " + pomPath)
		panic(err)
	}
	defer fi.Close()

	writer := bufio.NewWriter(fi)
	maven := JaxbMavenPomInfo{
		AppName: javaAppName,
	}
	err = t.Execute(writer, maven)
	if err != nil {
		log.Println("executing template:", err)
	}
	bufio.NewWriter(writer).Flush()
}

func printJavaJaxbMain(rootElementName string, javaDir string, javaPackage string, sourceXMLFilename string, date time.Time) {
	t := template.Must(template.New("chidleyJaxbGenClass").Parse(jaxbMainTemplate))
	writer, f, err := javaClassWriter(javaDir, javaPackage, "Main")
	defer f.Close()

	classInfo := JaxbMainClassInfo{
		PackageName:       javaPackage,
		BaseXMLClassName:  rootElementName,
		SourceXMLFilename: sourceXMLFilename,
		Date:              date,
	}
	err = t.Execute(writer, classInfo)
	if err != nil {
		log.Println("executing template:", err)
	}
	bufio.NewWriter(writer).Flush()

}

//func makeSourceReaders(sourceNames []string, url bool, standardIn bool) ([]Source, error) {
func makeSourceReaders(sourceNames []string, url bool, standardIn bool) (chan Source, error) {
	var err error
	//sources := make([]Source, len(sourceNames))
	sources := make(chan Source, 1)

	go func() {
		var newSource Source
		for i, _ := range sourceNames {
			if url {
				newSource = new(UrlSource)
				if DEBUG {
					log.Print("Making UrlSource")
				}
			} else {
				if standardIn {
					newSource = new(StdinSource)
					if DEBUG {
						log.Print("Making StdinSource")
					}
				} else {
					newSource = new(FileSource)
					if DEBUG {
						//log.Print("Making FileSource")
					}
				}
			}

			err = newSource.newSource(sourceNames[i])
			if err != nil {
				log.Fatal(err)
			}
			sources <- newSource
			if DEBUG {
				log.Print("Making Source:[" + sourceNames[i] + "]")
			}
		}
		close(sources)
	}()
	return sources, err
}

func attributes(atts map[string]bool) string {
	ret := ": "
	for k, _ := range atts {
		ret = ret + k + ", "
	}
	return ret
}

func indent(d int) string {
	indent := ""
	for i := 0; i < d; i++ {
		indent = indent + "\t"
	}
	return indent
}

func countNumberOfBoolsSet(a []*bool) int {
	counter := 0
	for i := 0; i < len(a); i++ {
		if *a[i] {
			counter += 1
		}
	}
	return counter
}

func makeOneLevelDown(node *Node, globalTagAttributes map[string]([]*FQN), flattenStrings, keepXmlFirstLetterCase bool, namePrefix, nameSuffix string) []*XMLType {
	var children []*XMLType

	for _, np := range node.children {
		if np == nil {
			continue
		}
		for _, n := range np.children {
			if n == nil {
				continue
			}
			if flattenStrings && isStringOnlyField(n, len(globalTagAttributes[nk(n)])) {
				continue
			}
			x := XMLType{NameType: n.makeType(namePrefix, nameSuffix, keepXmlFirstLetterCase),
				XMLName:      n.name,
				XMLNameUpper: capitalizeFirstLetter(n.name),
				XMLSpace:     n.space}
			children = append(children, &x)
		}
	}
	return children
}
func printChildrenChildren(node *Node) {
	for k, v := range node.children {
		log.Print(k)
		log.Printf("children: %+v\n", v.children)
	}
}

// Order Xml is encountered
func printStructsByXml(v *PrintGoStructVisitor) error {
	orderNodes := make(map[int]*Node)
	var order []int

	for k := range v.alreadyVisitedNodes {
		nodeOrder := v.alreadyVisitedNodes[k].discoveredOrder
		orderNodes[nodeOrder] = v.alreadyVisitedNodes[k]
		order = append(order, nodeOrder)
	}
	sort.Ints(order)

	for o := range order {
		err := print(v, orderNodes[o])
		if err != nil {
			return err
		}
	}
	return nil
}

// Alphabetical order
func printStructsAlphabetical(v *PrintGoStructVisitor) error {
	var keys []string
	for k := range v.alreadyVisitedNodes {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, k := range keys {
		err := print(v, v.alreadyVisitedNodes[k])
		if err != nil {
			return err
		}
	}
	return nil

}

func generateGoStructs(out io.Writer, sourceName string, ex *Extractor, flattenStrings, useType, keepXmlFirstLetterCase, nameSpaceInJsonName bool, namePrefix, nameSuffix, attributePrefix string) {
	printGoStructVisitor := new(PrintGoStructVisitor)

	printGoStructVisitor.init(os.Stdout, 999, ex.globalTagAttributes, ex.nameSpaceTagMap, flattenStrings, useType, nameSpaceInJsonName, namePrefix, nameSuffix, attributePrefix, keepXmlFirstLetterCase)
	printGoStructVisitor.Visit(ex.root)
	structSort(printGoStructVisitor)
}

//Writes structs to a string then uses this in a template to generate Go codes
func generateGoCode(out io.Writer, sourceNames []string, ex *Extractor, flattenStrings, useType, keepXmlFirstLetterCase, nameSpaceInJsonName bool, namePrefix, nameSuffix, attributePrefix string) error {
	buf := bytes.NewBufferString("")
	printGoStructVisitor := new(PrintGoStructVisitor)
	printGoStructVisitor.init(buf, 9999, ex.globalTagAttributes, ex.nameSpaceTagMap, flattenStrings, useType, nameSpaceInJsonName, namePrefix, nameSuffix, attributePrefix, keepXmlFirstLetterCase)
	printGoStructVisitor.Visit(ex.root)

	structSort(printGoStructVisitor)

	xt := XMLType{NameType: ex.firstNode.makeType(namePrefix, nameSuffix, keepXmlFirstLetterCase),
		XMLName:      ex.firstNode.name,
		XMLNameUpper: capitalizeFirstLetter(ex.firstNode.name),
		XMLSpace:     ex.firstNode.space,
	}

	fullPath, err := getFullPath(sourceNames[0])
	if err != nil {
		return err
	}

	fullPaths, err := getFullPaths(sourceNames)
	if err != nil {
		return err
	}
	x := XmlInfo{
		BaseXML:         &xt,
		OneLevelDownXML: makeOneLevelDown(ex.root, ex.globalTagAttributes, flattenStrings, keepXmlFirstLetterCase, namePrefix, nameSuffix),
		Filenames:       fullPaths,
		Filename:        fullPath,
		Structs:         buf.String(),
	}
	x.init()
	t := template.Must(template.New("chidleyGen").Parse(codeTemplate))

	err = t.Execute(out, x)
	if err != nil {
		log.Println("executing template:", err)
		return err
	}
	return err
}
