# changepoint [![PkgGoDev][godev-img]][godev] [![CI][ci-img]][ci]

Changepoint is a Go library for changepoint detection with support for
nonparametric distributions.

```go
package changepoint_test

import (
	"fmt"
	"math"
	"math/rand"

	"pgregory.net/changepoint"
)

func ExampleNonParametric() {
	r := rand.New(rand.NewSource(0))

	var data []float64
	for i := 0; i < 20; i++ {
		data = append(data, math.Exp(r.NormFloat64()+1))
	}
	for i := 0; i < 60; i++ {
		data = append(data, math.Exp(r.NormFloat64()))
	}
	for i := 0; i < 20; i++ {
		data = append(data, math.Exp(r.NormFloat64()-1))
	}

	fmt.Println(changepoint.NonParametric(data, 1))

	// Output:
	// [14 78]
}
```

## License

Changepoint is licensed under the [Apache License Version 2.0](./LICENSE).

ED-PELT implementation is based on the original
[Perfolizer](https://github.com/AndreyAkinshin/perfolizer)
code by Andrey Akinshin:

```
The MIT License

Copyright (c) 2020 Andrey Akinshin  
Copyright (c) 2013–2020 .NET Foundation and contributors

Permission is hereby granted, free of charge, to any person obtaining
a copy of this software and associated documentation files (the
"Software"), to deal in the Software without restriction, including
without limitation the rights to use, copy, modify, merge, publish,
distribute, sublicense, and/or sell copies of the Software, and to
permit persons to whom the Software is furnished to do so, subject to
the following conditions:

The above copyright notice and this permission notice shall be
included in all copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND,
EXPRESS OR IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF
MERCHANTABILITY, FITNESS FOR A PARTICULAR PURPOSE AND
NONINFRINGEMENT. IN NO EVENT SHALL THE AUTHORS OR COPYRIGHT HOLDERS BE
LIABLE FOR ANY CLAIM, DAMAGES OR OTHER LIABILITY, WHETHER IN AN ACTION
OF CONTRACT, TORT OR OTHERWISE, ARISING FROM, OUT OF OR IN CONNECTION
WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE SOFTWARE.
```

[godev-img]: https://pkg.go.dev/badge/pgregory.net/changepoint
[godev]: https://pkg.go.dev/pgregory.net/changepoint
[ci-img]: https://github.com/flyingmutant/changepoint/workflows/CI/badge.svg
[ci]: https://github.com/flyingmutant/changepoint/actions
