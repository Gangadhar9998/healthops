import { createContext, useContext, useState, useEffect, useCallback, type ReactNode } from 'react'

interface User {
  id: string
  username: string
  role: 'admin' | 'ops'
  displayName?: string
  createdAt: string
  updatedAt: string
}

interface AuthState {
  user: User | null
  token: string | null
  isAuthenticated: boolean
  isAdmin: boolean
  isLoading: boolean
}

interface AuthContextType extends AuthState {
  login: (username: string, password: string) => Promise<void>
  logout: () => void
}

const AuthContext = createContext<AuthContextType | null>(null)

const TOKEN_KEY = 'healthops_token'
const USER_KEY = 'healthops_user'

export function AuthProvider({ children }: { children: ReactNode }) {
  const [state, setState] = useState<AuthState>(() => {
    const token = localStorage.getItem(TOKEN_KEY)
    const userStr = localStorage.getItem(USER_KEY)
    let user: User | null = null
    try {
      if (userStr) user = JSON.parse(userStr)
    } catch { /* ignore */ }

    return {
      token,
      user,
      isAuthenticated: !!token && !!user,
      isAdmin: user?.role === 'admin',
      isLoading: !!token,
    }
  })

  // Verify token on mount
  useEffect(() => {
    if (!state.token) return
    
    fetch('/api/v1/auth/me', {
      headers: { Authorization: `Bearer ${state.token}` },
    })
      .then(res => {
        if (!res.ok) throw new Error('invalid token')
        return res.json()
      })
      .then(body => {
        const user = body.data?.user
        if (user) {
          setState(s => ({
            ...s,
            user,
            isAuthenticated: true,
            isAdmin: user.role === 'admin',
            isLoading: false,
          }))
          localStorage.setItem(USER_KEY, JSON.stringify(user))
        }
      })
      .catch(() => {
        localStorage.removeItem(TOKEN_KEY)
        localStorage.removeItem(USER_KEY)
        setState({ user: null, token: null, isAuthenticated: false, isAdmin: false, isLoading: false })
      })
  }, []) // eslint-disable-line react-hooks/exhaustive-deps

  const login = useCallback(async (username: string, password: string) => {
    const res = await fetch('/api/v1/auth/login', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ username, password }),
    })

    if (!res.ok) {
      const body = await res.json().catch(() => ({}))
      throw new Error(body?.error?.message || 'Login failed')
    }

    const body = await res.json()
    const { token, user } = body.data

    localStorage.setItem(TOKEN_KEY, token)
    localStorage.setItem(USER_KEY, JSON.stringify(user))

    setState({
      token,
      user,
      isAuthenticated: true,
      isAdmin: user.role === 'admin',
      isLoading: false,
    })
  }, [])

  const logout = useCallback(() => {
    localStorage.removeItem(TOKEN_KEY)
    localStorage.removeItem(USER_KEY)
    setState({ user: null, token: null, isAuthenticated: false, isAdmin: false, isLoading: false })
  }, [])

  return (
    <AuthContext.Provider value={{ ...state, login, logout }}>
      {children}
    </AuthContext.Provider>
  )
}

export function useAuth() {
  const ctx = useContext(AuthContext)
  if (!ctx) throw new Error('useAuth must be used within AuthProvider')
  return ctx
}
