import { Component, Show, JSX, createEffect, onCleanup } from 'solid-js'
import { Portal } from 'solid-js/web'

interface ModalProps {
  open: boolean
  onClose: () => void
  title?: string
  children: JSX.Element
  footer?: JSX.Element
  size?: 'sm' | 'md' | 'lg' | 'xl'
}

const sizeClasses = {
  sm: 'max-w-sm',
  md: 'max-w-md',
  lg: 'max-w-lg',
  xl: 'max-w-xl',
}

const Modal: Component<ModalProps> = (props) => {
  createEffect(() => {
    if (props.open) {
      document.body.style.overflow = 'hidden'
    } else {
      document.body.style.overflow = ''
    }
  })

  onCleanup(() => {
    document.body.style.overflow = ''
  })

  const handleBackdropClick = (e: MouseEvent) => {
    if (e.target === e.currentTarget) {
      props.onClose()
    }
  }

  const handleKeyDown = (e: KeyboardEvent) => {
    if (e.key === 'Escape') {
      props.onClose()
    }
  }

  createEffect(() => {
    if (props.open) {
      document.addEventListener('keydown', handleKeyDown)
    } else {
      document.removeEventListener('keydown', handleKeyDown)
    }
  })

  onCleanup(() => {
    document.removeEventListener('keydown', handleKeyDown)
  })

  return (
    <Show when={props.open}>
      <Portal>
        <div
          class="modal-overlay"
          onClick={handleBackdropClick}
        >
          <div class={`modal ${sizeClasses[props.size || 'md']}`}>
            <Show when={props.title}>
              <div class="modal-header flex items-center justify-between">
                <h3 class="text-lg font-semibold text-white">{props.title}</h3>
                <button
                  class="text-dark-400 hover:text-white transition-colors p-1 rounded-lg hover:bg-dark-800"
                  onClick={props.onClose}
                >
                  <svg class="w-5 h-5" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                    <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12" />
                  </svg>
                </button>
              </div>
            </Show>
            <div class="modal-body">{props.children}</div>
            <Show when={props.footer}>
              <div class="modal-footer">{props.footer}</div>
            </Show>
          </div>
        </div>
      </Portal>
    </Show>
  )
}

export default Modal
