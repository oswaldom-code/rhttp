.PHONY: report bench

report:
	go run ./report -count 5

bench:
	go test -bench=. -benchmem -count=5 -timeout=30m .
