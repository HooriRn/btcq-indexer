mkdir tmp/export

```sql:
\i docs/synthluvi/export.sql
```

```bash
cat tmp/export/units.csv tmp/export/depths.csv tmp/export/swaps.csv | sort > ./tmp/export/all.csv
python3 docs/synthluvi/synthluvi.py
```

Result looks like:
```
LUVI increase 1.0094172236467325
LUVIIncreaseDueToSynths:  1.0034495277111446
ValueChangePerUnit/LUVI 1.0038028332501272
```
