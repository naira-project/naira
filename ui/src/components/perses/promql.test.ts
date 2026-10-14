import { describe, expect, it } from 'vitest';
import { escapeLabelValue } from './promql';

describe('escapeLabelValue', () => {
  it.each([
    { name: 'empty string', input: '', want: '' },
    { name: 'plain value', input: 'idp-claude-sonnet', want: 'idp-claude-sonnet' },
    { name: 'bedrock model id', input: 'anthropic.claude-3-sonnet-20240229-v1:0', want: 'anthropic.claude-3-sonnet-20240229-v1:0' },
    { name: 'double quote', input: 'a"b', want: String.raw`a\"b` },
    { name: 'backslash', input: String.raw`a\b`, want: String.raw`a\\b` },
    { name: 'newline', input: 'a\nb', want: String.raw`a\nb` },
    { name: 'backslash followed by quote', input: String.raw`a\"b`, want: String.raw`a\\\"b` },
    { name: 'literal backslash-n is not a newline', input: String.raw`a\nb`, want: String.raw`a\\nb` },
    { name: 'trailing backslash cannot escape closing quote', input: 'a\\', want: 'a\\\\' },
    { name: 'multiple occurrences', input: '"a"\n"b"\n', want: String.raw`\"a\"\n\"b\"\n` },
    { name: 'label injection attempt', input: 'x", job="other', want: String.raw`x\", job=\"other` },
    { name: 'regex and braces are untouched', input: 'a.*{b}|c', want: 'a.*{b}|c' },
    { name: 'single quote is untouched', input: "it's", want: "it's" },
  ])('$name', ({ input, want }) => {
    expect(escapeLabelValue(input)).toBe(want);
  });
});
