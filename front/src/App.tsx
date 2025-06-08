import { createSignal, For, onCleanup, Setter, Show, type Component } from 'solid-js';

import styles from './App.module.css';
import { DurationInSeconds, formatDuration, formatTrack, Track } from './mpv';

interface SetGlobalPropertyBackendEvent {
    event: 'set-global-property';
    propertyName: string;
    value: any;
}

type BackendEvent = SetGlobalPropertyBackendEvent;

const App: Component<{ root: HTMLElement }> = ({ root }) => {
    const [connected, setConnected] = createSignal(false);
    const [playbackTime, setPlaybackTime] = createSignal<DurationInSeconds | null>(null);
    const [duration, setDuration] = createSignal<DurationInSeconds | null>(null);
    const [pause, setPause] = createSignal(false);
    const [volume, setVolume] = createSignal(100);
    const [path, setPath] = createSignal<string | null>(null);
    const [speed, setSpeed] = createSignal(1);
    const [ready, setReady] = createSignal(false);
    const [trackList, setTrackList] = createSignal<Track[] | null>(null);

    function selectedSubtitleTrackFromTrackList(trackList: Track[] | null): string {
        return formatTrack(trackList?.find(track => track.type === 'sub' && track.selected));
    }

    function selectedAudioTrackFromTrackList(trackList: Track[] | null): string {
        return formatTrack(trackList?.find(track => track.type === 'audio' && track.selected));
    }

    const selectedSubtitleTrack = (): string => selectedSubtitleTrackFromTrackList(trackList());
    const selectedAudioTrack = (): string => selectedAudioTrackFromTrackList(trackList());

    const globalProperties = new Map<string, Setter<unknown>>([
        ['connected', setConnected as any],
        ['playback-time', setPlaybackTime],
        ['duration', setDuration],
        ['pause', setPause],
        ['volume', setVolume],
        ['path', setPath],
        ['speed', setSpeed],
        ['ready', setReady],
        ['track-list', setTrackList]
    ]);

    function setGlobalProperty(propertyName: string, value: any): void {
        const setter = globalProperties.get(propertyName);
        if (!setter) {
            console.error(`Unknown global property "${propertyName}"`);
            return
        }
        setter(value);
    }

    function applyEvent(data: BackendEvent) {
        switch (data.event) {
            case 'set-global-property': {
                setGlobalProperty(data.propertyName, data.value);
                break;
            }

            default: {
                console.error(`Unknown event type "${data.event}"`);
                break;
            }
        }
    }

    const eventSource = new EventSource('/events');
    eventSource.onmessage = (event) => {
        console.log('RECV', event.data)

        applyEvent(JSON.parse(event.data));
    };

    onCleanup(() => {
        eventSource.close();
        document.removeEventListener('fullscreenchange', onFullscreenChange);
    });

    function command(args: any[]): Promise<Response> {
        console.log('command args', args);
        const body = new FormData();
        body.append('command', JSON.stringify(args));
        return fetch('/command', {
            method: 'POST',
            body: body,
        });
    }

    async function seek(change: number): Promise<void> {
        await command(['seek', change, 'relative+exact']);
        const playbackTimeResponse = await command(['get_property', 'playback-time']);
        const playbackTime = await playbackTimeResponse.json();
        await command(['show-text', formatDuration(playbackTime)]);
    }

    async function changeVolume(change: number): Promise<void> {
        const newVolume = volume() + change;
        await command(['set_property', 'volume', newVolume]);
        await command(['show-text', `Volume: ${newVolume}%`]);
    };

    async function changeSpeed(change: number): Promise<void> {
        let newSpeed = speed() + change;
        newSpeed = Math.round(newSpeed * 10) / 10;
        await command(['set_property', 'speed', newSpeed]);
        await command(['show-text', `Speed: ${newSpeed}`]);
    };

    function onFullscreenChange(e: Event): void {
        setInFullscreen(!!document.fullscreenElement);
    }

    document.addEventListener('fullscreenchange', onFullscreenChange);

    const [inFullscreen, setInFullscreen] = createSignal(false);
    async function toggleFullscreen(): Promise<void> {
        if (inFullscreen()) {
            await document.exitFullscreen();
        } else {
            await root.requestFullscreen({ navigationUI: 'hide' });
        }
    };

    interface FileSystemEntry {
        name: string;
        path: string;
        isDir: boolean;
    }

    interface FileSystemResponse {
        path: string;
        entries: FileSystemEntry[];
    }

    const [filePickerPath, setFilePickerPath] = createSignal('');
    const [filePickerEntries, setFilePickerEntries] = createSignal<FileSystemEntry[]>([]);

    let filePicker: HTMLDialogElement | undefined;

    async function openFilePicker(): Promise<void> {
        const response = await fetch(`/file-system?path=${encodeURIComponent(path() ?? '')}&dir=true`);
        const data = (await response.json()) as FileSystemResponse;

        setFilePickerPath(data.path);
        setFilePickerEntries(data.entries);

        filePicker?.showModal();
    }

    async function pickFile(entry: FileSystemEntry): Promise<void> {
        if (entry.isDir) {
            const response = await fetch(`/file-system?path=${encodeURIComponent(entry.path)}`);
            const data = (await response.json()) as FileSystemResponse;

            setFilePickerPath(data.path);
            setFilePickerEntries(data.entries);
        } else {
            await command(['loadfile', entry.path]);
            filePicker?.close();
        }
    };

    async function cycleSubtitleTrack(): Promise<void> {
        await command(['cycle', 'sub']);
        const response = await command(['get_property', 'track-list']);
        const trackList: Track[] | null = await response.json();
        await command(['show-text', `Subtitles: ${selectedSubtitleTrackFromTrackList(trackList)}`]);
    }

    async function cycleAudioTrack(): Promise<void> {
        await command(['cycle', 'audio']);
        const response = await command(['get_property', 'track-list']);
        const trackList: Track[] | null = await response.json();
        await command(['show-text', `Audio: ${selectedAudioTrackFromTrackList(trackList)}`]);
    }

    return (
        <Show when={ready()}>
            <Show
                when={connected()}
                fallback={
                    <div class={styles.notConnected}>
                        mpv is not connected

                        <form action="/" method="get">
                            <button class={styles.connect} type="submit">Connect</button>
                        </form>
                    </div>
                }
            >
                <svg style="display: none" xmlns="http://www.w3.org/2000/svg">
                    {/* Font Awesome Free 6.7.2 by @fontawesome - https://fontawesome.com License - https://fontawesome.com/license/free Copyright 2025 Fonticons, Inc. */}
                    <defs>
                        <symbol id="compress" viewBox="0 0 448 512">
                            <path style="fill: #fff" d="M160 64c0-17.7-14.3-32-32-32s-32 14.3-32 32l0 64-64 0c-17.7 0-32 14.3-32 32s14.3 32 32 32l96 0c17.7 0 32-14.3 32-32l0-96zM32 320c-17.7 0-32 14.3-32 32s14.3 32 32 32l64 0 0 64c0 17.7 14.3 32 32 32s32-14.3 32-32l0-96c0-17.7-14.3-32-32-32l-96 0zM352 64c0-17.7-14.3-32-32-32s-32 14.3-32 32l0 96c0 17.7 14.3 32 32 32l96 0c17.7 0 32-14.3 32-32s-14.3-32-32-32l-64 0 0-64zM320 320c-17.7 0-32 14.3-32 32l0 96c0 17.7 14.3 32 32 32s32-14.3 32-32l0-64 64 0c17.7 0 32-14.3 32-32s-14.3-32-32-32l-96 0z"/>
                        </symbol>
                        <symbol id="expand" viewBox="0 0 448 512">
                            <path style="fill: #fff" d="M32 32C14.3 32 0 46.3 0 64l0 96c0 17.7 14.3 32 32 32s32-14.3 32-32l0-64 64 0c17.7 0 32-14.3 32-32s-14.3-32-32-32L32 32zM64 352c0-17.7-14.3-32-32-32s-32 14.3-32 32l0 96c0 17.7 14.3 32 32 32l96 0c17.7 0 32-14.3 32-32s-14.3-32-32-32l-64 0 0-64zM320 32c-17.7 0-32 14.3-32 32s14.3 32 32 32l64 0 0 64c0 17.7 14.3 32 32 32s32-14.3 32-32l0-96c0-17.7-14.3-32-32-32l-96 0zM448 352c0-17.7-14.3-32-32-32s-32 14.3-32 32l0 64-64 0c-17.7 0-32 14.3-32 32s14.3 32 32 32l96 0c17.7 0 32-14.3 32-32l0-96z"/>
                        </symbol>
                    </defs>
                    <use href="#collapse"/>
                    <use href="#expand"/>
                </svg>

                <div class={styles.display}>
                    <div class={styles.topControls}>
                        <div
                            role="button"
                            class={styles.iconButton}
                            onClick={() => toggleFullscreen()}
                        >
                            <Show
                                when={inFullscreen()}
                                fallback={
                                    <svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 32 32"><use href="#expand"></use></svg>
                                }
                            >
                                <svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 32 32"><use href="#compress"></use></svg>
                            </Show>
                        </div>
                    </div>

                    <div class={styles.controls}>
                        <button
                            type="button"
                            onClick={() => seek(-10)}
                        >-10s</button>

                        <div></div>

                        <button
                            type="button"
                            onClick={() => seek(+10)}
                        >+10s</button>

                        <button
                            type="button"
                            onClick={() => changeVolume(-6)}
                        >-6v</button>

                        <button
                            type="button"
                            onClick={() => command(['set_property', 'pause', !pause()])}
                        >
                            {pause() ? 'Resume' : 'Pause'}
                        </button>

                        <button
                            type="button"
                            onClick={() => changeVolume(+6)}
                        >+6v</button>

                        <button
                            type="button"
                            onClick={() => changeSpeed(-0.1)}
                        >-0.1s</button>

                        <div></div>

                        <button
                            type="button"
                            onClick={() => changeSpeed(+0.1)}
                        >+0.1s</button>
                    </div>

                    <dialog ref={filePicker}>
                        <div>
                            <h3 style="margin-top: 0">{filePickerPath()}</h3>
                            <ul>
                                <For each={filePickerEntries()}>{entry =>
                                    <li>
                                        <div
                                            role="button"
                                            class={styles.link}
                                            onClick={event => { event.preventDefault(); pickFile(entry); }}
                                        >{entry.name}</div>
                                    </li>
                                }</For>
                            </ul>
                        </div>

                        <button type="button" onClick={() => filePicker?.close()}>Cancel</button>
                    </dialog>

                    <Show when={path()}>
                        <div>Current playback time: {formatDuration(playbackTime())} / {formatDuration(duration())}</div>
                        <div>
                            Audio: <div
                                role="button"
                                class={styles.link}
                                onClick={event => { event.preventDefault(); cycleAudioTrack(); }}
                            >{selectedAudioTrack()}</div>
                        </div>
                        <div>
                            Subtitles: <div
                                role="button"
                                class={styles.link}
                                onClick={event => { event.preventDefault(); cycleSubtitleTrack(); }}
                            >{selectedSubtitleTrack()}</div>
                        </div>
                    </Show>

                    <div>Volume: {volume()}% | Speed: {speed()}</div>
                    <div>
                        Path: <div
                            role="button"
                            class={styles.link}
                            onClick={event => { event.preventDefault(); openFilePicker(); }}
                        >{path() || 'No file selected'}</div>
                    </div>
                </div>
            </Show>
        </Show>
    )
};

export default App;
