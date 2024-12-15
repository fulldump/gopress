package api

import (
	"testing"
)

func Test_HtmlSummary(t *testing.T) {

	for _, c := range []struct {
		Input string
		Words int
		Want  string
	}{
		{
			Input: `<Div>
					<em>This is <b>the best article</b> ever written because of yes.</em>
			</div>`,
			Words: 4,
			Want:  `<em>This is <b>the best...</b></em>`,
		},
		{
			Input: `<Div>
					<em>This is <b>the best article</b> ever written because of yes.</em>
			</div>`,
			Words: 7,
			Want:  `<em>This is <b>the best article</b> ever written...</em>`,
		},
		{
			Input: `<Div>
					<em>This is <i><b>the best article</b></i> ever written because of yes.</em>
			</div>`,
			Words: 7,
			Want:  `<em>This is <i><b>the best article</b></i> ever written...</em>`,
		},
		{
			Input: `<Div>
					<em>This is <i><b>the best article</b></i> ever written because of yes.</em>
			</div>`,
			Words: 9999,
			Want:  `<em>This is <i><b>the best article</b></i> ever written because of yes.</em>`,
		},
		{
			Input: `<Div>
					This <b><em> is </b> <i>broken markup
			</div>`,
			Words: 9999,
			Want:  `This <b><em>is <i>broken markup</i></em></b>`,
		},
		{
			Input: `<Div>
					Go to <a href="https://gopress.org/hello" target="_blank">here</a>
			</div>`,
			Words: 9999,
			Want:  `Go to <a href="https://gopress.org/hello" target="_blank">here</a>`,
		},
	} {
		t.Run(c.Input, func(t *testing.T) {
			have := HtmlSummary(c.Input, c.Words)
			if have != c.Want {
				t.Errorf("HtmlSummary(%s, %d) = \n'%s'\n, want \n'%s'", c.Input, c.Words, have, c.Want)
			}
		})
	}

}
