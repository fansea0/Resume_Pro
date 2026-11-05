import { describe, expect, it } from 'vitest';

import { encodeWavPcm16, mixToMono, resample, TARGET_SAMPLE_RATE } from './wav';

describe('wav utils', () => {
  it('mixes channels into mono by averaging', () => {
    const left = new Float32Array([1, 0, -1]);
    const right = new Float32Array([0, 1, 1]);
    expect(Array.from(mixToMono([left, right]))).toEqual([0.5, 0.5, 0]);
  });

  it('resamples to the target rate', () => {
    const input = new Float32Array(1000).fill(0.5);
    const output = resample(input, 48000);
    expect(output.length).toBe(Math.round(1000 * (TARGET_SAMPLE_RATE / 48000)));
  });

  it('encodes a valid RIFF/WAVE header', () => {
    const samples = new Float32Array([0, 0.5, -0.5]);
    const buffer = encodeWavPcm16(samples, TARGET_SAMPLE_RATE);
    const view = new DataView(buffer);
    const tag = (offset: number) => String.fromCharCode(
      view.getUint8(offset), view.getUint8(offset + 1), view.getUint8(offset + 2), view.getUint8(offset + 3));
    expect(tag(0)).toBe('RIFF');
    expect(tag(8)).toBe('WAVE');
    expect(tag(12)).toBe('fmt ');
    expect(tag(36)).toBe('data');
    expect(view.getUint16(22, true)).toBe(1); // channels
    expect(view.getUint32(24, true)).toBe(TARGET_SAMPLE_RATE);
    expect(buffer.byteLength).toBe(44 + samples.length * 2);
  });

  it('declares the PCM format the backend validates', () => {
    const buffer = encodeWavPcm16(new Float32Array(4), TARGET_SAMPLE_RATE);
    const view = new DataView(buffer);
    expect(view.getUint32(4, true)).toBe(36 + 8); // RIFF 长度
    expect(view.getUint32(16, true)).toBe(16); // fmt 块长度
    expect(view.getUint16(20, true)).toBe(1); // PCM
    expect(view.getUint32(28, true)).toBe(TARGET_SAMPLE_RATE * 2); // byte rate
    expect(view.getUint16(32, true)).toBe(2); // block align
    expect(view.getUint16(34, true)).toBe(16); // 位深
    expect(view.getUint32(40, true)).toBe(8); // data 长度
  });

  it('scales samples into 16bit range and clamps the overflow', () => {
    const buffer = encodeWavPcm16(new Float32Array([1, -1, 0]), TARGET_SAMPLE_RATE);
    const view = new DataView(buffer);
    expect(view.getInt16(44, true)).toBe(32767);
    expect(view.getInt16(46, true)).toBe(-32768);
    expect(view.getInt16(48, true)).toBe(0);
  });
});
