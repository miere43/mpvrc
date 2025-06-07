export type DurationInSeconds = number;

export function formatDuration(seconds: DurationInSeconds | null | undefined): string {
    if (!seconds || seconds < 0) {
        return "00:00:00"
    }
    const hours = Math.floor(Math.floor(seconds) / 3600);
    const minutes = Math.floor((Math.floor(seconds) % 3600) / 60)
    return `${hours.toString().padStart(2, '0')}:${minutes.toString().padStart(2, '0')}:${(Math.floor(seconds) % 60).toString().padStart(2, '0')}`;
}

export interface TrackBase {
    id: number;
    title: string;
    lang: string;
    selected: boolean;
    default: boolean;
    forced: boolean;
}

export interface SubtitleTrack extends TrackBase {
    type: 'sub';
    codec: string;
}

export interface AudioTrack extends TrackBase {
    type: 'audio';
    codec?: string;
    decoder?: string;
    ['audio-channels']: number;
    ['demux-samplerate']: number;
}

export type Track = SubtitleTrack | AudioTrack;

export function formatTrack(track: Track | null | undefined): string {
    if (!track) {
        return 'no';
    }

    const parens: string[] = [track.lang];
    switch (track.type) {
        case 'sub':
            parens.push(track.codec);
            break;
        case 'audio':
            if (track.decoder) {
                parens.push(track.decoder);
            } else if (track.codec) {
                parens.push(track.codec);
            }
            parens.push(`${track['audio-channels']}ch`, `${track['demux-samplerate']} Hz`);
            break;
    }

    const square: string[] = [];
    if (track.default) {
        square.push('default');
    }
    if (track.forced) {
        square.push('forced');
    }

    let result = `(${track.id}) '${track.title}' (${parens.join(' ')})`;
    if (square.length > 0) {
        result += ` [${square.join(' ')}]`;
    }
    return result;
}
