import type {
  ContainerApp,
  ProjectContainerInfo,
  ProjectSecret,
  ProjectShare,
} from "../../models/project";

export interface ProjectDataLoadSignal {
  cancelled: boolean;
}

export interface ProjectContainerRecord {
  loading: boolean;
  data?: ProjectContainerInfo;
  error?: string;
  refreshedAt?: number;
}

export interface SecretsRecord {
  loading: boolean;
  data?: ProjectSecret[];
  error?: string;
}

export interface SharesRecord {
  loading: boolean;
  data?: ProjectShare[];
  /** Listening ports discovered in the container, used to offer share targets. */
  apps?: ContainerApp[];
  error?: string;
}

export interface AccessRecord {
  loading: boolean;
  data?: string[];
  error?: string;
}
