package cmd

import (
	"fmt"
	"os"

	"cosmossdk.io/log"
	"cosmossdk.io/store/metrics"
	"cosmossdk.io/store/types"
	dbm "github.com/cometbft/cometbft-db"
	"github.com/cometbft/cometbft/state"
	cmtstore "github.com/cometbft/cometbft/store"
	db "github.com/cosmos/cosmos-db"
	"github.com/syndtr/goleveldb/leveldb/opt"

	"github.com/binaryholdings/cosmos-pruner/internal/rootmulti"
)

// load db
// load app store and prune
// if immutable tree is not deletable we should import and export current state

func PruneAppState(dataDir string) error {

	o := opt.Options{
		DisableSeeksCompaction: true,
	}

	appDB, err := db.NewGoLevelDBWithOpts("application", dataDir, &o)
	if err != nil {
		return err
	}

	fmt.Println("pruning application state")

	appStore := rootmulti.NewStore(appDB, log.NewLogger(os.Stderr), metrics.NewNoOpMetrics())
	ver := rootmulti.GetLatestVersion(appDB)

	stores := []string{}
	if ver != 0 {
		cInfo, err := appStore.GetCommitInfo(ver)
		if err != nil {
			return err
		}

		stores = make([]string, len(cInfo.StoreInfos))
		for i, storeInfo := range cInfo.StoreInfos {
			stores[i] = storeInfo.Name
		}
	}

	keys := types.NewKVStoreKeys(stores...)
	for _, value := range keys {
		appStore.MountStoreWithDB(value, types.StoreTypeIAVL, nil)
	}

	err = appStore.LoadLatestVersion()
	if err != nil {
		return err
	}

	versions := appStore.GetAllVersions()

	v64 := make([]int64, len(versions))
	for i := 0; i < len(versions); i++ {
		v64[i] = int64(versions[i])
	}

	fmt.Println(len(v64))

	appStore.PruneStores(int64(len(v64)) - int64(keepVersions))

	fmt.Println("compacting application state")
	if err := appDB.ForceCompact(nil, nil); err != nil {
		return err
	}

	//create a new app store
	return nil
}

// PruneCmtData prunes the cometbft blocks and state based on the amount of blocks to keep
func PruneCmtData(dataDir string) error {

	o := opt.Options{
		DisableSeeksCompaction: true,
	}

	// Get BlockStore
	blockStoreDB, err := dbm.NewGoLevelDBWithOpts("blockstore", dataDir, &o)
	if err != nil {
		return err
	}
	blockStore := cmtstore.NewBlockStore(blockStoreDB)

	// Get StateStore
	stateDB, err := dbm.NewGoLevelDBWithOpts("state", dataDir, &o)
	if err != nil {
		return err
	}

	stateStore := state.NewStore(stateDB, state.StoreOptions{})

	base := blockStore.Base()

	pruneHeight := blockStore.Height() - int64(keepBlocks)

	state, err := stateStore.Load()
	if err != nil {
		return err
	}

	fmt.Println("pruning block store")
	// prune block store
	_, evidencePoint, err := blockStore.PruneBlocks(pruneHeight, state)
	if err != nil {
		return err
	}

	fmt.Println("compacting block store")
	if err := blockStoreDB.Compact(nil, nil); err != nil {
		return err
	}

	fmt.Println("pruning state store")
	// prune state store
	err = stateStore.PruneStates(base, pruneHeight, evidencePoint)
	if err != nil {
		return err
	}

	fmt.Println("compacting state store")
	if err := stateDB.Compact(nil, nil); err != nil {
		return err
	}

	return nil
}
