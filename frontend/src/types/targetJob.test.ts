import { describe, expectTypeOf, it } from 'vitest';

import type { TargetJob, TargetJobInput } from './targetJob';

describe('target-job type contracts', () => {
  it('keeps persisted metadata separate from the create/update payload', () => {
    expectTypeOf<TargetJob>().toEqualTypeOf<{
      id: number;
      company: string;
      position: string;
      requirements: string;
      createdAt: string;
      updatedAt: string;
    }>();
    expectTypeOf<TargetJobInput>().toEqualTypeOf<{
      company: string;
      position: string;
      requirements: string;
    }>();
  });
});
