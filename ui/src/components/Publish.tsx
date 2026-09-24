import * as Dialog from '@radix-ui/react-dialog'
import { useMutation } from '@tanstack/react-query'
import { Download, Share2, X } from 'lucide-react'
import { useState } from 'react'
import { api } from '../lib/api'
import { Button } from './ui'

// Publish signs a routine with the owner's author key and hands over the
// entry to propose to the gallery by pull request. Nothing is uploaded.
export function Publish({ id, name }: { id: string; name: string }) {
  const [open, setOpen] = useState(false)
  const [author, setAuthor] = useState('')
  const sign = useMutation({ mutationFn: () => api.publishRoutine(id, author.trim()) })
  const file = sign.data ? URL.createObjectURL(new Blob([JSON.stringify(sign.data, null, 1)], { type: 'application/json' })) : ''
  return (
    <Dialog.Root open={open} onOpenChange={(o) => { setOpen(o); if (!o) sign.reset() }}>
      <Dialog.Trigger asChild>
        <Button variant="ghost"><Share2 size={15} /> Publicar</Button>
      </Dialog.Trigger>
      <Dialog.Portal>
        <Dialog.Overlay className="fixed inset-0 z-40 bg-black/40 backdrop-blur-[2px]" />
        <Dialog.Content className="fixed left-1/2 top-[14vh] z-50 w-[min(520px,calc(100vw-32px))] -translate-x-1/2 rounded-2xl border border-line bg-surface p-6 shadow-[var(--shadow-pop)] focus:outline-none">
          <div className="mb-4 flex items-start justify-between gap-4">
            <div>
              <Dialog.Title className="text-[17px] font-semibold tracking-tight">Publicar “{name}” na galeria</Dialog.Title>
              <Dialog.Description className="mt-1 text-sm text-ink-2">Eu assino a rotina com a sua chave de autor. Quem instalar vê exatamente o código, as capacidades e os testes que você vê.</Dialog.Description>
            </div>
            <Dialog.Close className="grid size-8 shrink-0 place-items-center rounded-lg text-ink-3 hover:bg-sunken" aria-label="Fechar"><X size={16} /></Dialog.Close>
          </div>
          {sign.data ? (
            <div className="space-y-3 text-[13px] text-ink-2">
              <p>Assinada. Agora:</p>
              <ol className="list-decimal space-y-1 pl-5">
                <li>Baixe o arquivo abaixo.</li>
                <li>Abra um pull request em <a className="underline" href="https://github.com/denerFernandes/vigia-gallery" target="_blank" rel="noreferrer">vigia-gallery</a> com ele.</li>
                <li>A verificação roda sozinha; rotinas que enviam algo para outras pessoas passam por revisão humana.</li>
              </ol>
              <a href={file} download={`${id}.vigia.json`}><Button variant="primary"><Download size={15} /> Baixar {id}.vigia.json</Button></a>
            </div>
          ) : (
            <form className="space-y-3" onSubmit={(e) => { e.preventDefault(); sign.mutate() }}>
              <label className="block text-[13px] text-ink-2">Seu nome de autor
                <input autoFocus value={author} onChange={(e) => setAuthor(e.target.value)} placeholder="ex.: dener" className="mt-1.5 h-10 w-full rounded-[10px] border border-line bg-bg px-3 text-sm outline-none focus:border-accent" />
              </label>
              {sign.error && <p className="text-[13px] text-danger">{sign.error.message}</p>}
              <div className="flex justify-end"><Button variant="primary" type="submit" disabled={!author.trim() || sign.isPending}>Assinar</Button></div>
            </form>
          )}
        </Dialog.Content>
      </Dialog.Portal>
    </Dialog.Root>
  )
}
