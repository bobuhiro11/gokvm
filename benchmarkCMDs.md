go test -bench . <br />
go test -bench=FibInt <br />
go test -cpuprofile cpu.prof -memprofile mem.prof -bench . <br />
go test -cpuprofile cpu.prof -memprofile mem.prof -bench=FibInt <br />
go test -cpuprofile cpu.prof -memprofile mem.prof -bench=FibBig <br />

go tool pprof cpu.prof <br />
go tool pprof mem.prof <br />

go tool pprof -http=:8080 cpu.prof <br />
