package governor

import "context"

func Start(ctx context.Context) *Conn {
	return nil
}

type Conn struct {
}

func (c *Conn) Finish(ctx context.Context) {
}
