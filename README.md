# Generic cache with second chance algorithm

This package implements a cache uses the second hand algorithm with a bit map and
atomic operations. It allows fast and parallel `Get` operations. The `Has` method
test the presence of a key in the cache without altering its ejectable status.

A cache may be instantiated with the `New` method, otherwise it must be initialized
with the `Init` method. Using a non initialized cache will result in a panic. The
size of the cache may be change with a call to the `Init` method, but it will erase
the content of the cache. The `Reset` method also empties the cache.

The `Add` method adds a key value pair in the cache and `Delete` removes the pair
with the given key from the cache. When they return true, the returned value is the
deleted value which can then be recycled if desired. Unfortunately, they also lock
the cache to serialize these operations.

The method `Items` is an iterator on the all the key and value pairs in the cache.

The code is extensively tested with 100% coverage.

When a key value pair is deleted from the cache, the internal variables are cleared
to avoid a memory leak. Adding a key value pair doesn't require an allocation in the
cache doesn't require any allocation.

## Performance

The following tables show a summary of the performance. Add0 is a simple append
addition. Add1 is an addition with an ejection from the cache. Get0 is a `Get` with
a cache miss. Get1 is a `Get` with a cache hit. Has0 is a `Has` with a cache miss,
and Has1 is a `Has` with a cache hit. Note that `Has` doesn’t update the ejectable
status. The benchmarks were performed with a cache size of 10240.

| Macbook Air M2 |   Add0   |   Add1   |   Get0   |   Get1   |   Has0   |   Has1   |
|----------------|---------:|---------:|---------:|---------:|---------:|---------:|
| [int,int]      |     29ns |     54ns |     10ns |     13ns |     11ns |     11ns |
| [string,int]   |     55ns |     80ns |     13ns |     17ns |     12ns |     17ns |

| Intel i5 11thG |   Add0   |   Add1   |   Get0   |   Get1   |   Has0   |   Has1   |
|----------------|---------:|---------:|---------:|---------:|---------:|---------:|
| [int,int]      |     41ns |     66ns |     14ns |     18ns |     14ns |     14ns |
| [string,int]   |     70ns |    105ns |     15ns |     19ns |     15ns |     15ns |

Locating the key value pair to eject is fast because the bit map allows to test
64 positions in one operation. The use of atomic operations avoids the need to lock
the cache for serialized access with `Get` or `Has` calls. They use a `RWMutex` and
perform an `RLock`. Unfortunately the `Add`, `Delete` and `Items` calls must be
serialized by performing a `Lock`.

An RLU cache doesn’t allow parallel `Get` operations and is thus less performant
with multiple parallel applications. It is also slightly less memory efficient when
implemented with pointers.

## Installation

To install the package use the following command after the `go.mod` file is
initialized with `go mod init`.

    go get github.com/chmike/cache@latest
