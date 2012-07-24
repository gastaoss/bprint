package main

// The binary format specifier uses the same syntax as Ruby's Array.unpack
//
// c: signed 8-bit integer
// s: signed 16-bit integer
// l: signed 32-bit integer
// q: signed 65-bit integer
//
// Use upper case letter for unsigned integer.
//
// Numbers following the letter means how many times the previous string
// should be repeated.

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"flag"
	"fmt"
	"io"
	"os"
	"regexp"
	"strconv"
	"strings"
)

const version = "0.1"

func printVersion() {
	fmt.Println("bprint version", version)
	os.Exit(0)
}

var byteOrder = binary.LittleEndian

var (
	i8  int8
	i16 int16
	i32 int32
	i64 int64

	u8  uint8
	u16 uint16
	u32 uint32
	u64 uint64
)

type intType byte

type intDesc struct {
	typeId intType
	size   int
}

const noType intType = 255

const (
	I8 intType = iota
	I16
	I32
	I64

	U8
	U16
	U32
	U64
)

var descCharMap = map[byte]intDesc{
	'c': {I8, 1},
	's': {I16, 2},
	'l': {I32, 4},
	'q': {I64, 8},

	'C': {U8, 1},
	'S': {U16, 2},
	'L': {U32, 4},
	'Q': {U64, 8},
}

func isDigit(b byte) bool {
	return '0' <= b && b <= '9'
}

func parseBinaryFmtSpec(binFmt string) (formatDesc []intType, recSize int) {
	formatDesc = make([]intType, 0)
	var repeatNum int
	prevDesc := intDesc{noType, -1}
	for i := 0; i < len(binFmt); i++ {
		desc, ok := descCharMap[binFmt[i]]
		if ok {
			if repeatNum != 0 {
				// The original letter specifier is already added, so minus 1
				for i := 0; i < repeatNum-1; i++ {
					formatDesc = append(formatDesc, prevDesc.typeId)
				}
				recSize += (repeatNum - 1) * prevDesc.size
				repeatNum = 0
			}
			formatDesc = append(formatDesc, desc.typeId)
			prevDesc = desc
			recSize += desc.size
		} else {
			if isDigit(binFmt[i]) {
				if prevDesc.typeId == noType {
					// Number must follow a previous specifier
					panic("Data specifier error: repeat number without previous data specifier")
				}
				// Parse repeat number
				repeatNum = repeatNum*10 + int(binFmt[i]) - '0'
			} else {
				panic(fmt.Sprintf("Data specifier '%c' not supported", binFmt[i]))
			}
		}
	}
	// If the last specifier is a number
	for i := 0; i < repeatNum-1; i++ {
		formatDesc = append(formatDesc, prevDesc.typeId)
	}
	if repeatNum != 0 {
		recSize += (repeatNum - 1) * prevDesc.size
	}
	return
}

func readData(binReader io.Reader, formatDesc []intType, data []interface{}) (n int, err error) {
	for i, v := range formatDesc {
		switch v {
		case I8:
			err = binary.Read(binReader, byteOrder, &i8)
			data[i] = i8
		case I16:
			err = binary.Read(binReader, byteOrder, &i16)
			data[i] = i16
		case I32:
			err = binary.Read(binReader, byteOrder, &i32)
			data[i] = i32
		case I64:
			err = binary.Read(binReader, byteOrder, &i64)
			data[i] = i64

		case U8:
			err = binary.Read(binReader, byteOrder, &u8)
			data[i] = u8
		case U16:
			err = binary.Read(binReader, byteOrder, &u16)
			data[i] = u16
		case U32:
			err = binary.Read(binReader, byteOrder, &u32)
			data[i] = u32
		case U64:
			err = binary.Read(binReader, byteOrder, &u64)
			data[i] = u64
		}

		if err != nil {
			break
		}
		n++
	}
	return
}

var (
	recordCnt  int
	recordSize int
	offSet     int
)

const offsetFmt = "%07x "

func printData(printFmt string, data []interface{}) {
	if opt.printOffset {
		fmt.Printf(offsetFmt, offSet)
	}
	if opt.printRecordCnt {
		fmt.Printf("%d: ", recordCnt)
	}
	fmt.Printf(printFmt, data...)
}

func openFile(path string) (reader io.Reader, ioReader io.ReadCloser) {
	if path == "" {
		ioReader = os.Stdin
	} else {
		var err error
		ioReader, err = os.Open(path)
		if err != nil {
			fmt.Println("While opening file:", err)
			os.Exit(1)
		}
	}
	reader = bufio.NewReader(ioReader)
	return
}

const (
	defautlBinaryFmt = "C16"
)

var opt struct {
	printRecordCnt bool
	printOffset    bool
	printVersion   bool
	binaryFmt      string
	printFmt       string
}

func repeatWithSep(rep, sep string, cnt int) string {
	printFmt := strings.Repeat(rep+sep, cnt)
	return printFmt[:len(printFmt)-len(sep)]
}

func generatePrintFmt(cnt int, sep string) string {
	return repeatWithSep("%02x", sep, cnt)
}

func processPrintFmt(printFmt string) string {
	// Format like "%02d[sep]8#", "%d" will be repeated 8 times, with
	// seperator inserted
	printSpecPat, err := regexp.Compile("(%[^cdxo%]*[cdxo])([^\\d]*)(\\d+)#")
	if err != nil {
		panic(err)
	}
	mat := printSpecPat.FindAllStringSubmatchIndex(printFmt, -1)
	if mat == nil {
		return printFmt
	}

	buf := new(bytes.Buffer)
	prevIdx := 0
	for _, v := range mat {
		buf.WriteString(printFmt[prevIdx:v[0]])
		prevIdx = v[1]
		if v[0] > 0 && printFmt[v[0]-1] == '%' {
			// Do not parse spec following %%
			buf.WriteString(printFmt[v[0]:v[1]])
			continue
		}

		spec := printFmt[v[2]:v[3]]
		sep := printFmt[v[4]:v[5]]
		cntStr := printFmt[v[6]:v[7]]
		if sep == "" {
			sep = " "
		}
		cnt, err := strconv.Atoi(cntStr)
		if err != nil {
			panic(err)
		}

		buf.WriteString(repeatWithSep(spec, sep, cnt))
	}
	buf.WriteString(printFmt[prevIdx:])

	return buf.String()
}

func countPrintFmtSpec(printFmt string) int {
	specStr := "%[^cdxo%]*[cdxo]"
	// specStr must have a non-% preceeding or start from the beginning of line
	printSpecPat, err := regexp.Compile("([^%]{1}" + specStr + "|^" + specStr + ")")
	if err != nil {
		panic(err)
	}

	return len(printSpecPat.FindAllStringIndex(printFmt, -1))
}

func init() {
	flag.StringVar(&opt.binaryFmt, "e", defautlBinaryFmt,
		"binary format specifier. c,s,l,q for signed 8,16,32,64-bit int. Upper case for unsigned int")
	flag.StringVar(&opt.printFmt, "p", "",
		"printf style format string, size is implicit from binary format specifier, default to %02x for each field")
	flag.BoolVar(&opt.printVersion, "version", false,
		"print version information")
	flag.BoolVar(&opt.printRecordCnt, "c", false,
		"print record count")
	flag.BoolVar(&opt.printOffset, "o", false,
		"print record count")
}

func main() {
	defer func() {
		if err := recover(); err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
	}()
	flag.Parse()

	if opt.printVersion {
		printVersion()
	}

	binFilePath := flag.Arg(0)

	binReader, _ := openFile(binFilePath)

	formatDesc, recordSize := parseBinaryFmtSpec(opt.binaryFmt)
	formatDescLen := len(formatDesc)
	data := make([]interface{}, formatDescLen, formatDescLen)

	if opt.printFmt == "" {
		opt.printFmt = generatePrintFmt(formatDescLen, " ")
	}
	opt.printFmt = processPrintFmt(opt.printFmt)
	printSpecCnt := countPrintFmtSpec(opt.printFmt)
	if printSpecCnt != formatDescLen {
		panic(fmt.Sprintf("Binary spec has %d fields, print fmt has %d fields. Not match.",
			formatDescLen, printSpecCnt))
	}
	opt.printFmt += "\n"

	n := 0
	var err error
	for n, err = readData(binReader, formatDesc, data); err == nil; n, err = readData(binReader, formatDesc, data) {
		recordCnt++
		printData(opt.printFmt, data)
		offSet += recordSize
	}
	// Not enough data for the final line, print out what have been read
	if n != 0 {
		printData(opt.printFmt, data[0:n])
	} else if opt.printOffset {
		fmt.Printf(offsetFmt+"\n", offSet)
	}
	if err != io.EOF {
		if err == io.ErrUnexpectedEOF {
			fmt.Println("EOF: final data not enough for the last field")
		} else {
			fmt.Println("While reading data:", err)
		}
	}
}
