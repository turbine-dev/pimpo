import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Camera, KeyRound, Loader2, MapPin, Smartphone, Trash2 } from 'lucide-react'
import { useEffect, useRef, useState } from 'react'
import { api, type PhoneShare } from '../lib/api'
import { useT } from '../lib/i18n'
import { Button, Card } from '../components/ui'

const field = 'h-10 w-full rounded-[10px] border border-line bg-bg px-3 text-sm outline-none focus:border-accent'

// Phone makes a paired phone part of the agent: what it shares (chosen on
// the phone), the owner's places, photos for routines to read, and a key
// for automations on the phone.
export function Phone() {
  const t = useT()
  const qc = useQueryClient()
  const q = useQuery({ queryKey: ['phone'], queryFn: api.phone })
  const done = () => qc.invalidateQueries({ queryKey: ['phone'] })
  const device = q.data?.device
  const shares = device?.shares ?? []
  const setShares = useMutation({ mutationFn: api.phoneShares, onSuccess: done })
  const addPlace = useMutation({ mutationFn: api.addPlace, onSuccess: done })
  const delPlace = useMutation({ mutationFn: api.deletePlace, onSuccess: done })
  const photo = useMutation({ mutationFn: api.phonePhoto })
  const key = useMutation({ mutationFn: api.phoneKey, onSuccess: done })
  const [name, setName] = useState('')
  const [where, setWhere] = useState<string | null>(null)
  const last = useRef(0)

  // While this page is open on a phone that shares its location, Pimpo
  // hears where it is, at most once a minute; the position is not kept.
  const sharesLocation = shares.includes('location')
  useEffect(() => {
    if (!sharesLocation || !navigator.geolocation) return
    const id = navigator.geolocation.watchPosition((p) => {
      if (Date.now() - last.current < 60_000) return
      last.current = Date.now()
      api.phoneLocation(p.coords.latitude, p.coords.longitude, p.coords.accuracy).then((r) => setWhere(r.at.join(', ') || null)).catch(() => {})
    }, () => {}, { enableHighAccuracy: true, maximumAge: 60_000 })
    return () => navigator.geolocation.clearWatch(id)
  }, [sharesLocation])

  const toggle = (s: PhoneShare, on: boolean) => setShares.mutate(on ? [...shares, s] : shares.filter((x) => x !== s))
  const here = () => navigator.geolocation?.getCurrentPosition(
    (p) => addPlace.mutate({ name: name.trim(), lat: p.coords.latitude, lon: p.coords.longitude }),
    () => addPlace.mutate({ name: name.trim() }),
  )
  const origin = window.location.origin

  return (
    <div className="mx-auto max-w-3xl space-y-4">
      <div>
        <h1 className="mb-1 flex items-center gap-2 text-[22px] font-semibold tracking-tight"><Smartphone size={20} /> {t('node.title')}</h1>
        <p className="text-sm text-ink-2">{t('node.text')}</p>
      </div>

      <Card className="space-y-3 p-4">
        <h2 className="text-[15px] font-medium">{t('node.sharesTitle')}</h2>
        {!device && <p className="text-[13px] text-ink-2">{t('node.notPhone')}</p>}
        {device && (q.data?.shares ?? []).map((s) => (
          <label key={s} className="flex items-start gap-3 text-[14px]">
            <input type="checkbox" className="mt-1" checked={shares.includes(s)} onChange={(e) => toggle(s, e.target.checked)} />
            <span><span className="font-medium">{t(`node.share.${s}`)}</span><span className="block text-[12.5px] text-ink-3">{t(`node.share.${s}Hint`)}</span></span>
          </label>
        ))}
        {sharesLocation && where && <p className="text-[12.5px] text-ink-2"><MapPin size={12} className="mr-1 inline" />{t('node.at', { place: where })}</p>}
      </Card>

      {device && shares.includes('camera') && (
        <Card className="space-y-3 p-4">
          <h2 className="text-[15px] font-medium">{t('node.photoTitle')}</h2>
          <p className="text-[13px] text-ink-2">{t('node.photoText')}</p>
          <label className="inline-flex cursor-pointer items-center gap-2 rounded-[10px] bg-ink px-4 py-2 text-[14px] font-medium text-bg hover:opacity-90">
            {photo.isPending ? <Loader2 size={16} className="animate-spin" /> : <Camera size={16} />} {t('node.takePhoto')}
            <input type="file" accept="image/*" capture="environment" className="sr-only" onChange={(e) => e.target.files?.[0] && photo.mutate(e.target.files[0])} />
          </label>
          {photo.data && <p className="whitespace-pre-wrap rounded-[10px] bg-sunken p-3 text-[12.5px] text-ink-2">{photo.data.text || photo.data.note || t('node.photoNoText')}</p>}
          {photo.error && <p className="text-[13px] text-danger">{photo.error.message}</p>}
        </Card>
      )}

      <Card className="space-y-3 p-4">
        <h2 className="text-[15px] font-medium">{t('node.placesTitle')}</h2>
        <p className="text-[13px] text-ink-2">{t('node.placesText')}</p>
        {(q.data?.places ?? []).map((p) => (
          <div key={p.name} className="flex items-center gap-2 text-[14px]">
            <MapPin size={14} className="text-ink-3" />
            <span className="flex-1">{p.name} <span className="text-[12px] text-ink-3">{p.lat || p.lon ? t('node.radius', { m: Math.round(p.radius ?? 150) }) : t('node.byAutomation')}</span></span>
            <Button size="sm" variant="ghost" aria-label={t('node.removePlace', { name: p.name })} onClick={() => delPlace.mutate(p.name)}><Trash2 size={14} /></Button>
          </div>
        ))}
        <form className="flex flex-wrap gap-2" onSubmit={(e) => { e.preventDefault(); addPlace.mutate({ name: name.trim() }) }}>
          <input className={`${field} min-w-[180px] flex-1`} value={name} onChange={(e) => setName(e.target.value)} placeholder={t('node.placeName')} aria-label={t('node.placeName')} />
          <Button type="button" disabled={!name.trim() || addPlace.isPending} onClick={here}><MapPin size={14} /> {t('node.here')}</Button>
          <Button type="submit" variant="ghost" disabled={!name.trim() || addPlace.isPending}>{t('node.nameOnly')}</Button>
        </form>
        {addPlace.error && <p className="text-[13px] text-danger">{addPlace.error.message}</p>}
      </Card>

      {device && (
        <Card className="space-y-3 p-4">
          <h2 className="flex items-center gap-2 text-[15px] font-medium"><KeyRound size={15} /> {t('node.keyTitle')}</h2>
          <p className="text-[13px] text-ink-2">{t('node.keyText')}</p>
          <Button size="sm" onClick={() => key.mutate()} disabled={key.isPending}>{device.has_key ? t('node.keyNew') : t('node.keyMake')}</Button>
          {key.data && (
            <div className="space-y-2 rounded-[10px] bg-sunken p-3 text-[12.5px]">
              <p className="font-medium text-change">{t('node.keyOnce')}</p>
              <code className="block break-all">{key.data.key}</code>
              <p className="text-ink-2">{t('node.keyHow')}</p>
              <code className="block break-all">POST {origin}/api/phone/arrived · Authorization: Bearer … · {'{"place": "Casa"}'}</code>
              <code className="block break-all">POST {origin}/api/phone/left · {'{"place": "Casa"}'}</code>
              <code className="block break-all">POST {origin}/api/phone/shortcut · {'{"name": "…", "text": "…"}'}</code>
            </div>
          )}
        </Card>
      )}
    </div>
  )
}
