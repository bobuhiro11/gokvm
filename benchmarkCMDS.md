go test -bench .
go test -bench=FibInt
go test -cpuprofile cpu.prof -memprofile mem.prof -bench .
go test -cpuprofile cpu.prof -memprofile mem.prof -bench=FibInt
go test -cpuprofile cpu.prof -memprofile mem.prof -bench=FibBig

go tool pprof cpu.prof
go tool pprof mem.prof

go tool pprof -http=:8080 cpu.prof
