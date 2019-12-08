package taskq

type noCopy struct{}

func (*noCopy) Lock()   {}
func (*noCopy) Unlock() {}
