To test the performance of the newly implemented batched block
fetching and DB inserting, and parallel block fetching, I did a very
simple unscientific benchmark. With a completely synced Midgard:

- trim back 200000 blocks from the DB
- set the parameters and sync back up
- measuring the timings, RAM and CPU usages

Thornode, PostgreSQL and Midgard were all running on the same machine:

4 Core Intel i7-3770 CPU @ 3.40GHz
16GB DDR3 RAM
Crucial MX500 SATA SSD

Date: 2021-07-18

|----------------+------------------+-----------------+------------------+------------------+------------------+-------------+--------------|
| FetchBatchSize | FetchParallelism | InsertBatchSize | Block fetch (ms) | Block write (ms) | Midgard RAM (MB) | Midgard CPU | Thornode CPU |
|----------------+------------------+-----------------+------------------+------------------+------------------+-------------+--------------|
|              0 |                0 |               0 |                  |                  |                  |             |          40% |
|             20 |                1 |              20 |                7 |              1.6 |               48 |         80% |          55% |
|            100 |                1 |             100 |              6.5 |             1.06 |               64 |         84% |          50% |
|           1000 |                1 |             137 |             6.27 |             0.97 |              164 |         75% |          60% |
|           3000 |                1 |             337 |             6.15 |             0.88 |              360 |        100% |         100% |
|----------------+------------------+-----------------+------------------+------------------+------------------+-------------+--------------|
|            100 |                2 |              87 |              3.6 |             1.15 |               80 |        150% |          90% |
|            300 |                2 |             137 |             3.47 |             1.07 |              105 |        150% |          90% |
|           1000 |                2 |             337 |              3.3 |             0.97 |              165 |        150% |          90% |
|           3000 |                2 |             337 |              3.3 |             0.97 |              380 |        150% |         110% |
|----------------+------------------+-----------------+------------------+------------------+------------------+-------------+--------------|
|            100 |                4 |              87 |              2.4 |              1.4 |               80 |        260% |         150% |
|            300 |                4 |             137 |              2.3 |              1.3 |              110 |        280% |         160% |
|           1000 |                4 |             337 |              2.3 |              1.2 |              230 |        290% |         160% |
|           3000 |                4 |             337 |              2.1 |              1.3 |              400 |        300% |         180% |
|----------------+------------------+-----------------+------------------+------------------+------------------+-------------+--------------|
|            102 |                6 |              87 |                2 |              1.7 |               77 |        360% |         200% |
|            300 |                6 |             137 |             1.95 |             1.66 |              110 |        380% |         210% |
|           1002 |                6 |             337 |              1.9 |              1.5 |              240 |        390% |         230% |
|           3000 |                6 |             337 |              1.9 |              1.6 |              480 |        380% |         240% |
|----------------+------------------+-----------------+------------------+------------------+------------------+-------------+--------------|

With all of the components running on the same machine the test
results are not very authoritative. But some key take-aways:

- Larger batch sizes improve performance, but not very significantly,
  parallelism is much more important
- Midgard uses around 1MB of RAM per 10 blocks (currently)
- Midgard syncing parallelizes well with available cores, but uses a
  lot of CPU to process and prepare data for inserting into the DB. To
  be investigated…
