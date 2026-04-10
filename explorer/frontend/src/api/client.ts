const BASE = "/api/v1";

async function get<T>(path: string): Promise<T> {
  const res = await fetch(`${BASE}${path}`);
  if (!res.ok) {
    const err = await res.json().catch(() => ({ error: res.statusText }));
    throw new Error(err.error || res.statusText);
  }
  return res.json();
}

export interface Dashboard {
  height: number;
  totalBlocks: number;
  totalTxs: number;
  recentTxs: TxRecord[];
}

export interface TxRecord {
  txId: string;
  heightHex: string;
  blockNum: number;
  txNum: number;
  status: number;
  statusName: string;
}

export interface TxList {
  transactions: TxRecord[];
  total: number;
}

export interface TxDetail {
  txId: string;
  heightHex: string;
  blockNum: number;
  txNum: number;
  status: number;
  statusName: string;
  channelId?: string;
  timestamp?: string;
  type?: string;
  namespaces?: TxNamespace[];
  endorsers?: Endorser[];
}

export interface TxNamespace {
  nsId: string;
  nsVersion: number;
  reads?: TxRead[];
  readWrites?: TxReadWrite[];
  blindWrites?: TxWrite[];
}

export interface TxRead {
  key: string;
  version: number | null;
  keyLabel?: string;
}

export interface TxReadWrite {
  key: string;
  version: number | null;
  value: string;
  keyLabel?: string;
  valueInfo?: string;
}

export interface TxWrite {
  key: string;
  value: string;
  keyLabel?: string;
  valueInfo?: string;
}

export interface Endorser {
  mspId: string;
  subject?: string;
}

export interface DecodedBlock {
  number: number;
  dataHash: string;
  previousHash: string;
  txCount: number;
  transactions: BlockTx[];
}

export interface BlockTx {
  index: number;
  txId: string;
  type: string;
  timestamp: string;
  channelId: string;
}

export interface StateRow {
  key: string;
  value: string;
  version: number;
}

export interface StateResponse {
  namespace: string;
  rows: StateRow[];
}

export interface NamespacesResponse {
  namespaces: string[];
}

export const api = {
  dashboard: () => get<Dashboard>("/dashboard"),
  transactions: (limit = 20, offset = 0) => get<TxList>(`/transactions?limit=${limit}&offset=${offset}`),
  tx: (txid: string) => get<TxDetail>(`/transactions/${txid}`),
  block: (num: number) => get<DecodedBlock>(`/blocks/${num}`),
  namespaces: () => get<NamespacesResponse>("/namespaces"),
  state: (ns: string, limit = 100) => get<StateResponse>(`/state/${ns}?limit=${limit}`),
};
