package main

type repeatableStringFlag []string

func (f *repeatableStringFlag) String() string {
	if f == nil {
		return ""
	}
	out := ""
	for i, v := range *f {
		if i > 0 {
			out += ","
		}
		out += v
	}
	return out
}

func (f *repeatableStringFlag) Set(value string) error {
	*f = append(*f, value)
	return nil
}
