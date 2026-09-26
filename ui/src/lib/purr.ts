// purr plays a cat's purr, made on the spot with Web Audio: soft noise,
// low-passed into a rumble and pulsed about 26 times a second, in breaths
// that swell on the way in and out. No recording is shipped. It only
// plays after the owner does something (feeding), so browsers allow it.
export function purr(seconds = 3.2, volume = 0.22) {
  const Ctx = window.AudioContext ?? (window as unknown as { webkitAudioContext?: typeof AudioContext }).webkitAudioContext
  if (!Ctx) return
  const ctx = new Ctx()
  const now = ctx.currentTime
  const noise = ctx.createBuffer(1, ctx.sampleRate * seconds, ctx.sampleRate)
  const data = noise.getChannelData(0)
  for (let i = 0; i < data.length; i++) data[i] = Math.random() * 2 - 1
  const src = ctx.createBufferSource()
  src.buffer = noise

  const low = ctx.createBiquadFilter()
  low.type = 'lowpass'
  low.frequency.value = 320
  low.Q.value = 0.8

  // The flutter: a gain opened and closed ~26 times a second.
  const flutter = ctx.createGain()
  flutter.gain.value = 0.5
  const lfo = ctx.createOscillator()
  lfo.type = 'sine'
  lfo.frequency.value = 26
  const depth = ctx.createGain()
  depth.gain.value = 0.5
  lfo.connect(depth).connect(flutter.gain)

  // The breaths: in louder, out softer, each under a second.
  const breath = ctx.createGain()
  breath.gain.setValueAtTime(0, now)
  const cycle = 0.9
  for (let t = 0; t < seconds; t += cycle) {
    breath.gain.linearRampToValueAtTime(volume, now + t + cycle * 0.25)
    breath.gain.linearRampToValueAtTime(volume * 0.25, now + t + cycle * 0.5)
    breath.gain.linearRampToValueAtTime(volume * 0.7, now + t + cycle * 0.75)
    breath.gain.linearRampToValueAtTime(volume * 0.1, now + t + cycle)
  }
  breath.gain.linearRampToValueAtTime(0, now + seconds)

  src.connect(low).connect(flutter).connect(breath).connect(ctx.destination)
  src.start(now)
  lfo.start(now)
  src.stop(now + seconds)
  lfo.stop(now + seconds)
  src.onended = () => ctx.close()
}
