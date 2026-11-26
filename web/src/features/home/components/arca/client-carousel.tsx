/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import { Pause, Play } from 'lucide-react'
import { useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import {
  Carousel,
  CarouselContent,
  CarouselItem,
  CarouselNext,
  CarouselPrevious,
  type CarouselApi,
} from '@/components/ui/carousel'

const clients = [
  {
    name: 'Codex',
    protocol: 'OpenAI Responses',
    logo: 'openai.svg',
    monochrome: true,
  },
  {
    name: 'Claude Code',
    protocol: 'Claude Messages',
    logo: 'claude-color.svg',
  },
  {
    name: 'OpenCode',
    protocol: 'OpenAI / Claude',
    logo: 'opencode.svg',
    monochrome: true,
  },
  { name: 'Gemini CLI', protocol: 'Gemini API', logo: 'gemini-color.svg' },
  {
    name: 'Cline',
    protocol: 'OpenAI Chat Completions',
    logo: 'cline.svg',
    monochrome: true,
  },
] as const

export function ClientCarousel() {
  const { t } = useTranslation()
  const region = useRef<HTMLDivElement>(null)
  const [carousel, setCarousel] = useState<CarouselApi>()
  const [selected, setSelected] = useState(0)
  const [paused, setPaused] = useState(false)
  const [visible, setVisible] = useState(true)
  const [hovered, setHovered] = useState(false)
  const [focused, setFocused] = useState(false)
  const [reducedMotion, setReducedMotion] = useState(
    () =>
      window.matchMedia?.('(prefers-reduced-motion: reduce)').matches ?? false
  )
  const rotating = visible && !paused && !hovered && !focused && !reducedMotion

  useEffect(() => {
    if (!region.current || typeof IntersectionObserver === 'undefined') return
    const observer = new IntersectionObserver(([entry]) => {
      setVisible(entry?.isIntersecting ?? true)
    })
    observer.observe(region.current)
    return () => observer.disconnect()
  }, [])

  useEffect(() => {
    const media = window.matchMedia?.('(prefers-reduced-motion: reduce)')
    if (!media) return
    const onChange = (event: MediaQueryListEvent) =>
      setReducedMotion(event.matches)
    media.addEventListener('change', onChange)
    return () => media.removeEventListener('change', onChange)
  }, [])

  useEffect(() => {
    if (!carousel) return
    const onSelect = () => setSelected(carousel.selectedScrollSnap())
    onSelect()
    carousel.on('select', onSelect)
    carousel.on('reInit', onSelect)
    return () => {
      carousel.off('select', onSelect)
      carousel.off('reInit', onSelect)
    }
  }, [carousel])

  useEffect(() => {
    if (!carousel || !rotating) return
    const timer = window.setInterval(() => carousel.scrollNext(), 4800)
    return () => window.clearInterval(timer)
  }, [carousel, rotating])

  return (
    <Carousel
      ref={region}
      aria-label={t('Supported Applications')}
      className='arca-client-carousel'
      opts={{ loop: true, skipSnaps: false, duration: reducedMotion ? 0 : 25 }}
      setApi={setCarousel}
      onPointerEnter={() => setHovered(true)}
      onPointerLeave={() => setHovered(false)}
      onFocusCapture={() => setFocused(true)}
      onBlurCapture={(event) => {
        if (!event.currentTarget.contains(event.relatedTarget)) {
          setFocused(false)
        }
      }}
    >
      <CarouselContent className='ml-0' aria-live={rotating ? 'off' : 'polite'}>
        {clients.map((client, index) => (
          <CarouselItem
            key={client.name}
            className='pl-0 sm:basis-1/3'
            aria-label={client.name}
            data-active={index === selected}
          >
            <div className='arca-client-card'>
              <img
                src={`/brand/${client.logo}`}
                alt=''
                className={
                  'monochrome' in client
                    ? 'arca-client-icon arca-client-icon-mono'
                    : 'arca-client-icon'
                }
                width='64'
                height='64'
                draggable={false}
              />
              <h2>{client.name}</h2>
              <p>{client.protocol}</p>
            </div>
          </CarouselItem>
        ))}
      </CarouselContent>
      <div className='arca-carousel-actions'>
        <CarouselPrevious
          variant='ghost'
          className='static size-10 translate-y-0'
          aria-label={t('Previous client')}
        />
        <Button
          type='button'
          size='icon'
          variant='ghost'
          className='size-10'
          aria-label={paused ? t('Resume carousel') : t('Pause carousel')}
          disabled={reducedMotion}
          onClick={() => setPaused((value) => !value)}
        >
          {paused ? <Play aria-hidden='true' /> : <Pause aria-hidden='true' />}
        </Button>
        <CarouselNext
          variant='ghost'
          className='static size-10 translate-y-0'
          aria-label={t('Next client')}
        />
      </div>
    </Carousel>
  )
}
