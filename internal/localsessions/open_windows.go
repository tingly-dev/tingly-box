package localsessions

import "os"

func openReadOnly(root *os.Root, rel string) (*os.File, error) { return root.Open(rel) }
