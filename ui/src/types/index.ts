export interface TrackedResource {
  uid: string;
  apiVersion: string;
  kind: string;
  namespace: string;
  name: string;
  currentRevision: number;
  firstSeen: string;
  lastSeen: string;
  isDeleted: boolean;
  policyName: string;
}

export interface ResourceRevision {
  id: number;
  resourceUid: string;
  apiVersion: string;
  kind: string;
  namespace: string;
  name: string;
  revision: number;
  eventType: 'CREATED' | 'UPDATED' | 'DELETED';
  isSnapshot: boolean;
  resourceVersion: string;
  changedFields: string[] | null;
  timestamp: string;
  policyName: string;
  sizeBytes: number;
  content?: Record<string, unknown>;
}

export interface StorageStats {
  totalResources: number;
  totalRevisions: number;
  storageBytes: number;
  oldestRevision: string | null;
  newestRevision: string | null;
}

export interface KindStats {
  apiVersion: string;
  kind: string;
  resources: number;
  revisions: number;
}

export interface DiffResult {
  fromRevision: number;
  toRevision: number;
  patch: Record<string, unknown>;
  changedPaths: string[];
  fromSnapshot: Record<string, unknown>;
  toSnapshot: Record<string, unknown>;
}

export interface AISummary {
  revision: number;
  summary: string;
}

export interface AIDiffSummary {
  fromRevision: number;
  toRevision: number;
  summary: string;
}

export interface Anomaly {
  severity: 'high' | 'medium' | 'low';
  title: string;
  description: string;
  resource: string;
  revision: number;
}

export interface AnomalyReport {
  anomalies: Anomaly[];
  summary: string;
}

export interface QueryResult {
  answer: string;
  sources?: { kind: string; name: string; namespace: string; revision?: number }[];
}

export interface ApiResponse<T> {
  data: T;
  error?: string;
  meta?: {
    total: number;
    limit: number;
    offset: number;
  };
}
