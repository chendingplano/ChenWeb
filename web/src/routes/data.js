// ChenWeb/web/src/routes/data.js
export function getData() {
  const countries = [
    { id: 'us', name: 'United States' },
    { id: 'ca', name: 'Canada' },
    { id: 'uk', name: 'United Kingdom' },
    { id: 'de', name: 'Germany' },
    { id: 'fr', name: 'France' }
  ];

  const users = Array.from({ length: 100 }, (_, i) => ({
    id: i + 1,
    name: `User ${i + 1}`,
    email: `user${i + 1}@example.com`,
    country: ['us', 'ca', 'uk', 'de', 'fr'][i % 5],
    active: i % 3 !== 0
  }));

  return {
    allData: users,
    countries,
    users
  };
}
