package main

/*
json: cannot unmarshal object into Go struct field PublicKey.results.validator_updates.pub_key.Sum of type crypto.isPublicKey_Sum, cannot read block
	9793, 23114, 38596, 52998, 89720, 250282, 297804, 341006, 384208, 427410, 470613, 513816, 557019, 600222, 695988, 739193, 782396, 825600, 869523, 912727, 956651, 999859, 1043064, 1086268, 1130913, 1175558, 1218763, 1261969, 1307335, 1354149, 1445597, 1445627
*/

/*
//Reusable template for debugging record parsing
func TestUnmarshal_SetNodeMimir(t *testing.T) {
	bytes, err := ioutil.ReadFile("block_3776131_set_node_mimir.json")
	assert.Nil(t, err)
	assert.NotNil(t, bytes)
	var block chain.Block
	err = tmjson.Unmarshal(bytes, &block)
	assert.Nil(t, err)
	actual := record.SetNodeMimir{}
	err = actual.LoadTendermint(block.Results.TxsResults[0].Events[3].Attributes)
	assert.Nil(t, err)
	expected := record.SetNodeMimir{
		Address: []byte("thor165rm26dtenzkryhv5waxay3ey3mayg4sjf7w69"),
		Key:     3,
		Value:   []byte("VALIDATORMAXREWARDRATIO"),
	}
	assert.Equal(t, expected, actual)
}
*/
