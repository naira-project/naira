import { render, screen } from '@testing-library/react';
import App from './App';

test('renders the Naira application shell', () => {
  render(<App />);
  expect(screen.queryByText(/learn react/i)).not.toBeInTheDocument();
});
