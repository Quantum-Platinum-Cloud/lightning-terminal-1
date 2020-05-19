import { observable } from 'mobx';
import { IS_DEV, IS_TEST } from 'config';
import AppStorage from 'util/appStorage';
import { actionLog, Logger } from 'util/log';
import { GrpcClient, LndApi, LoopApi } from 'api';
import {
  BuildSwapStore,
  ChannelStore,
  NodeStore,
  SettingsStore,
  SwapStore,
  UiStore,
} from './stores';
import { PersistentSettings } from './stores/settingsStore';

/**
 * The store used to manage global app state
 */
export class Store {
  //
  // Child Stores
  //
  buildSwapStore = new BuildSwapStore(this);
  channelStore = new ChannelStore(this);
  swapStore = new SwapStore(this);
  nodeStore = new NodeStore(this);
  settingsStore = new SettingsStore(this);
  uiStore = new UiStore(this);

  /** the backend api services to be used by child stores */
  api: {
    lnd: LndApi;
    loop: LoopApi;
  };

  /** the logger for actions to use when modifying state */
  log: Logger;

  /** the wrapper class around persistent storage */
  storage: AppStorage<PersistentSettings>;

  // a flag to indicate when the store has completed all of its
  // API requests requested during initialization
  @observable initialized = false;

  constructor(
    lnd: LndApi,
    loop: LoopApi,
    storage: AppStorage<PersistentSettings>,
    log: Logger,
  ) {
    this.api = { lnd, loop };
    this.log = log;
    this.storage = storage;
  }

  /**
   * load initial data to populate the store
   */
  async init() {
    this.settingsStore.init();
    await this.channelStore.fetchChannels();
    await this.swapStore.fetchSwaps();
    await this.nodeStore.fetchBalances();
    this.initialized = true;
  }
}

/**
 * Creates an initialized Store instance with the dependencies injected
 * @param grpcClient an alternate GrpcClient to use instead of the default
 */
export const createStore = (
  grpcClient?: GrpcClient,
  appStorage?: AppStorage<PersistentSettings>,
) => {
  const grpc = grpcClient || new GrpcClient();
  const storage = appStorage || new AppStorage();
  const lndApi = new LndApi(grpc);
  const loopApi = new LoopApi(grpc);

  const store = new Store(lndApi, loopApi, storage, actionLog);
  // initialize the store immediately to fetch API data, except when running unit tests
  if (!IS_TEST) store.init();

  // in dev env, make the store accessible via the browser DevTools console
  if (IS_DEV) (global as any).store = store;

  return store;
};
