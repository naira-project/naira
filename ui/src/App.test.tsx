import { render, screen, act } from '@testing-library/react';
import App from './App';

test('renders the Naira application shell', async () => {
  await act(async () => {
    render(<App />);
  });
  expect(screen.queryByText(/learn react/i)).not.toBeInTheDocument();
});
