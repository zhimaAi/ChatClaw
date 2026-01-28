package greet

type GreetService struct {
	prefix string
}

func NewGreetService(prefix string) *GreetService {
	return &GreetService{prefix: prefix}
}

func (g *GreetService) Greet(name string) string {
	return g.prefix + name + "!"
}

