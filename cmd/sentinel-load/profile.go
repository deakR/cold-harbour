package main

func writeProfiles(dir string, work func() error) error {
	return work()
}
