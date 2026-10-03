//go:build !linux

package containment

import "os"

type Group struct{}

func OpenRoot(string) (*os.File, error) { return nil, ErrUnavailable }
func New(string, int64) (*Group, error) { return nil, ErrUnavailable }
func (*Group) FD() int                  { return -1 }
func (*Group) Kill() error              { return ErrUnavailable }
func (*Group) Close() error             { return ErrUnavailable }

func (*Group) OOMKilled() (bool, error) { return false, ErrUnavailable }
