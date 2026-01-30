# BlockStore

- [BlockStore](#blockstore)
  - [Summary](#summary)
    - [Goals](#goals)
  - [Challenges](#challenges)
    - [JSON is slow](#json-is-slow)
    - [Serializing/deserializing interface fields](#serializingdeserializing-interface-fields)
    - [Defining and compiling proto definitions is a hassle](#defining-and-compiling-proto-definitions-is-a-hassle)
  - [Format](#format)
    - [Compressed tendermint JSON](#compressed-tendermint-json)
    - ["Textual" Gobs](#textual-gobs)
    - [Seekable binary Gobs](#seekable-binary-gobs)

## Summary

We need a simple format for a permanent storage of thornode blocks. It would be used for:

1. Speeding up Midgard sync and removing the corresponding load from Thornodes

2. Preserving historical blocks which can be used for syncing Midgard and debugging even after
   a Thornode fork that trims history.

### Goals

1. Plain files in a directory which can be easily rsynced, wgeted, sha256summed, etc.

2. Preferably, format that can be manipulated and processed with standard command line tools,
   like `wc`, `head`, `tail`, `jq`, etc.

3. Deterministic: dumps done by different users should be bit-for-bit equal.

4. Complete: should contain all of the info returned by the Thornode's `BlockResults` API call,
   not just the parts that are useful for Midgard. (So it can be used later for any purpose.)

5. Preferably a format that allows for (relatively) fast lookup of an individual block. (To be
   served by Midgard on the `/v2/debug/block/NNNNN` endpoint.)

6. Self-descriptive: looking at the files in a BlockStore directory it should be easy to infer
   which files contain what.

Non-goals:

1. Format that allows for real-time querying and processing. This is only intended for archival and
   fast sequential reading purposes (and limited single-block lookup as described above.)

## Challenges

Before describing the proposed solutions we describe the biggest technical challenges we found:

### JSON is slow

JSON would be an obvious choice for the basis of the BlockStore format, since it's human readable,
can be processed by standard tools, and is the format that we get the data in from the Thornode
API to begin with.

Unfortunately, JSON parsing implementations in Go are very slow. One `BlockResults` from Chaosnet
at heights around 3'000'000 is a JSON object of average 22 kB size. Parsing it into a Go struct
takes around 4-5ms. We have tried the following JSON implementations, and none of them were
faster than 4 ms/block on the test machine:

- `github.com/cometbft/cometbft/libs/json`
- standard `encodings/json`
- `github.com/json-iterator/go`
- `github.com/segmentio/encoding/json`
- `github.com/goccy/go-json`
- `github.com/mailru/easyjson`

As a comparison, all of the binary serialization libraries that we've tried are at least an order
of magnitude faster (measured with a layer of `base64` encoding and `zsdt` compression):

- `encoding/gob`: 0.43 ms/block
- `github.com/fxamacker/cbor/v2`: 0.36 ms/block
- `github.com/gogo/protobuf/proto`: 0.16 ms/block

For historical documentation of these benchmarks see the `historical-blockstore-encodings`
branch.

### Serializing/deserializing interface fields

One of the fields of the `chain.Block` data structure, in particular
`Results.ValidatorUpdates.PubKey.Sum` is of a (private) interface type in the
`github.com/cometbft/cometbft/proto/tendermint/crypto` package. This presents a problem for
serializing/deserializing it.

It can be natively handled with the tendermint's json package
`github.com/cometbft/cometbft/libs/json`, as it's aware of tendermint's types and uses
reflection to handle it properly. It can also be handled by the protobuf library; it's actually
defined as a `one_of` protobuf field, and Go's protobuf implementation translates it into an
interface type.

None of the other tested libraries can handle this field: different JSON implementation, Go's
native `gob` binary serialization, or `github.com/fxamacker/cbor/v2` CBOR implementation.

### Defining and compiling proto definitions is a hassle

The `tendermint` library has proto definitions for most of its data structures, but for some
reason not for the `ResultBlockResults`, which is the main part of our `Block` struct. So we
would need to redefine it as a proto message and keep it in sync with `tendermint`.

The tools compiling `proto` definitions into Go files do not integrate well with Go's building
philosophy, as dependencies have to be present locally (cannot just `import` from
`github.com/tendermint/...`), so it requires an additional piece of infrastructure to maintain.

## Format

Taking into account the main requirements and the obstacles described in the
[Challenges](#challenges) section we chose the following format.

### Compressed tendermint JSON

This is the format we have implemented originally. It's been obsoleted for the "slow JSON decoding"
reason, but we leave it here as it's a good intermediary/export format.

1. Blocks are stored in individual files containing chunks of 10000 blocks (by default,
   configurable.) The files are named with the height of the last block stored in them, in decimal,
   padded with zeroes to 12 digits.

2. The files are stream compressed with `zstd` at level 1 (by default, configurable). This results
   in compressed size of around 2.4% of the original.

3. The stream contains compactly JSON encoded blocks (using tendermint's JSON encoding,
   `github.com/cometbft/cometbft/libs/json`), one block per line.

This format does not properly satisfy the requirement that a random block should be fast to look up,
since accessing a random block within a chunk file requires decompressing and reading through
half of the file on average. That's why the 10000 block per file number was chosen, it takes less
than 0.1 ms to decompressing files of this size.

### "Textual" Gobs


This format preserves all of the features of the format above, except every
line in the stream is `base64` encoded `gob` encoded block.

To overcome the challenge describe in the
[Serializing/deserializing interface fields](#serializingdeserializing-interface-fields) section,
we can introduce a struct that extends `chain.Block` but stores the problematic field separately
encoded with Tendermint's JSON. (Blocks with this field present are very rare, so this does not
incur a significant overhead.)

Using a textual line-oriented format does not provide significant advantages if the lines are just
binary blobs, so we have the option to replace it with a proper binary format. This will
also allow using a format that allows for random access.

### Seekable binary Gobs

TBD.
