export type DurationInSeconds = number;

export function formatDuration(seconds: DurationInSeconds | null | undefined): string {
    if (!seconds || seconds < 0) {
        return "00:00:00"
    }
    const hours = Math.floor(Math.floor(seconds) / 3600);
    const minutes = Math.floor((Math.floor(seconds) % 3600) / 60)
    return `${hours.toString().padStart(2, '0')}:${minutes.toString().padStart(2, '0')}:${(Math.floor(seconds) % 60).toString().padStart(2, '0')}`;
}
