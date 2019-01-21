package main

type structSortFunc func(v *PrintGoStructVisitor)

var structSort = printStructsAlphabetical

var DEBUG = false
var progress = false
