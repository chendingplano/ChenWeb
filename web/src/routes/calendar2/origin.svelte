<script lang="ts">
    import { onMount } from 'svelte';

  // Props
  let { width = 300, height = 360 } = $props<{ width?: number; height?: number }>();



    let monthYearEl: HTMLDivElement | null = null;
    let prevButtonEl: HTMLDivElement | null = null;
    let nextButtonEl: HTMLDivElement | null = null;

    const months = [
        'January', 'February', 'March', 'April', 'May', 'June',
        'July', 'August', 'September', 'October', 'November', 'December'
    ];

    let currentDate = $state(new Date());
    const today = new Date();

    type DayInfo = {
        text: string;
        className: string;
    };

    let days = $state<DayInfo[]>([]);

    $effect(() => {
        // It creates the calendar days whenever currentDate changes.
        // In a given month, there are days from the previous and next months to fill the calendar grid.
        // Days in the current month are highlighted, and today's date is specially marked.
        // Days from other months are styled with a 'fade' class.
        // These are captured by 'className' in DayInfo.
        const date = currentDate;
        const year = date.getFullYear();
        const month = date.getMonth();
        const firstDay = new Date(year, month, 1).getDay();
        const lastDay = new Date(year, month + 1, 0).getDate();

        if (monthYearEl) {
            monthYearEl.textContent = `${months[month]} ${year}`;
        }

        const result: DayInfo[] = [];

        // Previous month
        const prevMonthLastDay = new Date(year, month, 0).getDate();
        for (let i = firstDay; i > 0; i--) {
            result.push({
                text: String(prevMonthLastDay - i + 1),
                className: 'fade'
            });
        }

        // Current month
        for (let i = 1; i <= lastDay; i++) {
            let className = '';
            if (i === today.getDate() && month === today.getMonth() && year === today.getFullYear()) {
                className = 'today';
            }
            result.push({ text: String(i), className });
        }

        // Next month
        const nextMonthStartDay = 7 - new Date(year, month + 1, 0).getDay() - 1;
        for (let i = 1; i <= nextMonthStartDay; i++) {
            result.push({
                text: String(i),
                className: 'fade'
            });
        }

        // Update the reactive state
        days = result;
        });

    onMount(() => {
        // Initial render is handled by $derived + template
        prevButtonEl?.addEventListener('click', () => {
          currentDate = new Date(currentDate.getFullYear(), currentDate.getMonth() - 1, 1);
        });

        nextButtonEl?.addEventListener('click', () => {
          currentDate = new Date(currentDate.getFullYear(), currentDate.getMonth() + 1, 1);
        });
    });
</script>

<link rel="stylesheet" href="https://cdnjs.cloudflare.com/ajax/libs/font-awesome/6.5.2/css/all.min.css" />

<div class="calendar">
  <div class="header">
    <div bind:this={prevButtonEl} class="btn"><i class="fa-solid fa-arrow-left"></i></div>
    <div bind:this={monthYearEl} id="month-year"></div>
    <div bind:this={nextButtonEl} class="btn"><i class="fa-solid fa-arrow-right"></i></div>
  </div>
  <div class="weekdays">
    <div>Sun</div>
    <div>Mon</div>
    <div>Tue</div>
    <div>Wed</div>
    <div>Thu</div>
    <div>Fri</div>
    <div>Sat</div>
  </div>
  <div class="days">
    {#each days as day}
      <div class={day.className}>{day.text}</div>
    {/each}
  </div>
</div>

<style>
  /* Only body needs to be global */
  :global(body) {
    font-family: Arial, sans-serif;
    display: grid;
    place-items: center;
    height: 100vh;
    margin: 0;
    background-color: #333333;
  }

  .calendar {
    background-color: #111111;
    border-radius: 10px;
    box-shadow: 0 0 30px #222222;
    overflow: hidden;
    width: 300px;
    color: white;
    padding: 20px;
  }
  .header {
    display: flex;
    justify-content: space-between;
    align-items: center;
    padding: 10px;
  }
  .btn {
    cursor: pointer;
  }
  #month-year {
    font-weight: bold;
    font-size: 20px;
  }
  .weekdays, .days {
    display: flex;
    flex-wrap: wrap;
  }
  .days {
    height: 210px;
  }
  .weekdays div,
  .days div {
    width: 14.28%;
    text-align: center;
    padding: 10px 0;
    border-radius: 5px;
  }
  .days div {
    cursor: pointer;
    transition: background-color 0.3s;
  }
  .days div:hover {
    background-color: white;
    color: orangered;
  }
  .days .today {
    background-color: orangered;
    color: white;
  }
  .days .fade {
    color: #555;
  }
</style>
