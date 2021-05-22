package cacheutil

// func DistributeShards(newCluster *CacheCluster, cluster *CacheCluster, args *CacheClusterArgs) []*Shard {
// 	cluster.Mux.Lock()
// 	defer cluster.Mux.Unlock()

// 	if cluster.ShardsAmount < args.ShardsAmount {
// 		diff := args.ShardsAmount - cluster.ShardsAmount

// 		for addingShardIdx := 0; addingShardIdx < diff; addingShardIdx++ {
// 			newShard := CreateShard(args.ShardSize*mbBytes, args.CachePolicy)

// 			for shardIdx := range cluster.shards {

// 				for hashedKey := range cluster.shards[shardIdx].Hashmap {
// 					newShardIdx := jumpConsistentHash(hashedKey, args.ShardsAmount)
// 					if newShardIdx != int64(args.ShardsAmount-addingShardIdx-1) {
// 						continue
// 					}

// 					shardItem := cluster.shards[shardIdx].Hashmap[hashedKey]
// 					TTL := cluster.shards[shardIdx].Policy.HashMap[hashedKey].TTL

// 					newShard.set(hashedKey, cluster.shards[shardIdx].Items[shardItem.Index], TTL)
// 					cluster.shards[shardIdx].delete(hashedKey, shardItem.Index, shardItem.Length)
// 				}

// 			}

// 			newCluster.shards = append(newCluster.shards, newShard)
// 		}
// 	}

// 	if cluster.ShardsAmount > args.ShardsAmount {
// 		diff := cluster.ShardsAmount - args.ShardsAmount

// 		for removingShardIdx := 1; removingShardIdx <= diff; removingShardIdx++ {

// 			for hashedKey := range cluster.shards[cluster.ShardsAmount-removingShardIdx].Hashmap {
// 				newShardIdx := jumpConsistentHash(hashedKey, cluster.ShardsAmount-removingShardIdx-1)

// 				shardItem := cluster.shards[cluster.ShardsAmount-removingShardIdx].Hashmap[hashedKey]
// 				TTL := cluster.shards[cluster.ShardsAmount-removingShardIdx].Policy.HashMap[hashedKey].TTL

// 				newCluster.shards[newShardIdx].set(hashedKey, cluster.shards[newCluster.ShardsAmount-removingShardIdx].Items[shardItem.Index], TTL)
// 			}

// 			cluster.shards[newCluster.ShardsAmount-removingShardIdx] = nil
// 			cluster.shards = cluster.shards[:newCluster.ShardsAmount-removingShardIdx]
// 		}
// 	}

// 	// TODO: resize shards as well
// 	newCluster.ShardSize = args.ShardSize
// 	newCluster.ShardsAmount = len(newCluster.shards)

// 	debug.SetGCPercent(GCPercentRatio(args.ShardsAmount, args.ShardSize))

// 	return newCluster.shards
// }
