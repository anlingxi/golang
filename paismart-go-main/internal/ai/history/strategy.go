package history

type ReadStrategy string
type WriteStrategy string

const (
	ReadStrategyCacheThenDB ReadStrategy = "cache_then_db"
	ReadStrategyDBOnly      ReadStrategy = "db_only"
)

const (
	WriteStrategyCacheAndAsyncDB WriteStrategy = "cache_and_async_db"
	WriteStrategySyncDBOnly      WriteStrategy = "sync_db_only"
	WriteStrategySyncDoubleWrite WriteStrategy = "sync_double_write"
)

type Strategy struct {
	Read  ReadStrategy
	Write WriteStrategy
}
