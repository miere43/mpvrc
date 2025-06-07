import { expect, test, describe } from 'vitest';
import { AudioTrack, formatDuration, formatTrack, SubtitleTrack } from './mpv';

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
});

describe('formatTrack', ()  => {
    const subtitleTrack1: SubtitleTrack & Record<string, any> = {
        "id": 1,
        "type": "sub",
        "src-id": 4,
        "title": "Signs \u0026 Songs [neoDESU]",
        "lang": "en",
        "image": false,
        "albumart": false,
        "default": true,
        "forced": true,
        "dependent": false,
        "visual-impaired": false,
        "hearing-impaired": false,
        "external": false,
        "selected": false,
        "ff-index": 3,
        "codec": "ass",
        "codec-desc": "Advanced Sub Station Alpha",
        "metadata": {
            "BPS": "151",
            "DURATION": "00:23:07.270000000",
            "NUMBER_OF_FRAMES": "203",
            "NUMBER_OF_BYTES": "26204",
            "_STATISTICS_WRITING_APP": "mkvmerge v79.0 ('Funeral Pyres') 64-bit",
            "_STATISTICS_WRITING_DATE_UTC": "2023-09-22 19:52:04",
            "_STATISTICS_TAGS": "BPS DURATION NUMBER_OF_FRAMES NUMBER_OF_BYTES"
        }
    };

    const subtitleTrack2: SubtitleTrack & Record<string, any> = {
        "id": 2,
        "type": "sub",
        "src-id": 5,
        "title": "Full Subtitles [Commie]",
        "lang": "en",
        "image": false,
        "albumart": false,
        "default": false,
        "forced": false,
        "dependent": false,
        "visual-impaired": false,
        "hearing-impaired": false,
        "external": false,
        "selected": true,
        "main-selection": 0,
        "ff-index": 4,
        "codec": "ass",
        "codec-desc": "Advanced Sub Station Alpha",
        "metadata": {
            "BPS": "247",
            "DURATION": "00:23:07.250000000",
            "NUMBER_OF_FRAMES": "511",
            "NUMBER_OF_BYTES": "42954",
            "_STATISTICS_WRITING_APP": "mkvmerge v79.0 ('Funeral Pyres') 64-bit",
            "_STATISTICS_WRITING_DATE_UTC": "2023-09-22 19:52:04",
            "_STATISTICS_TAGS": "BPS DURATION NUMBER_OF_FRAMES NUMBER_OF_BYTES"
        }
    };

    const audioTrack1: AudioTrack & Record<string, any> = {
        "id": 1,
        "type": "audio",
        "src-id": 2,
        "title": "English FLAC 2.0",
        "lang": "en",
        "audio-channels": 2,
        "image": false,
        "albumart": false,
        "default": true,
        "forced": false,
        "dependent": false,
        "visual-impaired": false,
        "hearing-impaired": false,
        "external": false,
        "selected": true,
        "main-selection": 0,
        "ff-index": 1,
        "decoder": "flac",
        "decoder-desc": "FLAC (Free Lossless Audio Codec)",
        "codec": "flac",
        "codec-desc": "FLAC (Free Lossless Audio Codec)",
        "demux-channel-count": 2,
        "demux-channels": "stereo",
        "demux-samplerate": 48000,
        "metadata": {
            "BPS": "646753",
            "DURATION": "00:22:52.373000000",
            "NUMBER_OF_FRAMES": "14296",
            "NUMBER_OF_BYTES": "110948433",
            "_STATISTICS_WRITING_APP": "mkvmerge v79.0 ('Funeral Pyres') 64-bit",
            "_STATISTICS_WRITING_DATE_UTC": "2023-09-22 19:52:04",
            "_STATISTICS_TAGS": "BPS DURATION NUMBER_OF_FRAMES NUMBER_OF_BYTES"
        }
    };

    const audioTrack2: AudioTrack & Record<string, any> = {
        "id": 2,
        "type": "audio",
        "src-id": 3,
        "title": "Japanese FLAC 2.0",
        "lang": "ja",
        "audio-channels": 2,
        "image": false,
        "albumart": false,
        "default": false,
        "forced": false,
        "dependent": false,
        "visual-impaired": false,
        "hearing-impaired": false,
        "external": false,
        "selected": false,
        "ff-index": 2,
        "codec": "flac",
        "demux-channel-count": 2,
        "demux-channels": "unknown2",
        "demux-samplerate": 48000,
        "metadata": {
            "BPS": "610628",
            "DURATION": "00:23:22.070000000",
            "NUMBER_OF_FRAMES": "14605",
            "NUMBER_OF_BYTES": "107018015",
            "_STATISTICS_WRITING_APP": "mkvmerge v79.0 ('Funeral Pyres') 64-bit",
            "_STATISTICS_WRITING_DATE_UTC": "2023-09-22 19:52:04",
            "_STATISTICS_TAGS": "BPS DURATION NUMBER_OF_FRAMES NUMBER_OF_BYTES"
        }
    };

    for (const { name, track, want } of [
        {
            name: '"null" must convert to "no"',
            track: null,
            want: 'no',
        },
        {
            name: '"undefined" must convert to "no"',
            track: undefined,
            want: 'no',
        },
        {
            track: subtitleTrack1,
            want:  "(1) 'Signs & Songs [neoDESU]' (en ass) [default forced]",
        },
        {
            track: subtitleTrack2,
            want:  "(2) 'Full Subtitles [Commie]' (en ass)",
        },
        {
            track: audioTrack1,
            want: "(1) 'English FLAC 2.0' (en flac 2ch 48000 Hz) [default]",
        },
        {
            track: audioTrack2,
            want: "(2) 'Japanese FLAC 2.0' (ja flac 2ch 48000 Hz)",
        },
    ]) {
        test(name ?? want, () => { expect(formatTrack(track)).toBe(want); });
    }
})

