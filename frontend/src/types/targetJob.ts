export interface TargetJob {
  id: number;
  company: string;
  position: string;
  requirements: string;
  createdAt: string;
  updatedAt: string;
}

export interface TargetJobInput {
  company: string;
  position: string;
  requirements: string;
}
