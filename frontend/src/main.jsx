import React from 'react'
import ReactDOM from 'react-dom/client'
import App from './App.jsx'
import './styles.css'

window.BUILD_INFO = window.BUILD_INFO || { commit: 'dev', time: '' }

ReactDOM.createRoot(document.getElementById('root')).render(<App />)
