import { expect, test, describe } from 'vitest';
import { formatDuration } from './duration';

describe('formatDuration', () => {
    for (const { seconds, want } of [
        {
            seconds: null,
            want:    "00:00:00",
        },
        {
            seconds: undefined,
            want:    "00:00:00",
        },
        {
            seconds: 4.004000,
            want:    "00:00:04",
        },
        {
            seconds: 0,
            want:    "00:00:00",
        },
        {
            seconds: 1,
            want:    "00:00:01",
        },
        {
            seconds: 59,
            want:    "00:00:59",
        },
        {
            seconds: 60,
            want:    "00:01:00",
        },
        {
            seconds: 61,
            want:    "00:01:01",
        },
        {
            seconds: 3599,
            want:    "00:59:59",
        },
        {
            seconds: 3600,
            want:    "01:00:00",
        },
        {
            seconds: 3661,
            want:    "01:01:01",
        },
        {
            seconds: 86399,
            want:    "23:59:59",
        },
        {
            seconds: 86400,
            want:    "24:00:00",
        },
        {
            seconds: -1,
            want:    "00:00:00",
        },
        {
            seconds: 3723.7,
            want:    "01:02:03",
        },
    ]) {
        test(`${seconds} seconds must convert to ${want}`, () => {
            expect(formatDuration(seconds)).toBe(want);
        });
    }
})

